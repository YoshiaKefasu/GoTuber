package tweaks

import (
	"image/color"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Panel は Tweaks パネルの ebitenui UI 実装。
type Panel struct {
	ui    *ebitenui.UI
	state *State
}

// NewPanel は ebitenui を使った Tweaks パネルを構築する。
// face: ロード済み *text.GoTextFace (Gen Interface JP Regular)
// state: パネルの状態を保持する State
// audioEnabled: マイクが利用可能な場合 true。false のとき Audio checkbox は操作不可。
func NewPanel(face *text.GoTextFace, state *State, audioEnabled bool) *Panel {
	p := &Panel{state: state}
	// ebitenui の *text.Face 期待 API に対応するため、interface 値をローカル変数に保持してアドレス取得
	faceIface := text.Face(face)
	facePtr := &faceIface

	// ボタンテキストカラー
	btnTextColor := &widget.ButtonTextColor{
		Idle:     color.NRGBA{0xdf, 0xf4, 0xff, 0xff},
		Disabled: color.NRGBA{0x99, 0x99, 0x99, 0xff},
	}
	labelColorIdle := color.NRGBA{0xcc, 0xcc, 0xcc, 0xff}
	labelColorDim := color.NRGBA{0x88, 0x88, 0x88, 0xff}

	// ルートコンテナ: 縦並び + パディング 16
	root := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(color.NRGBA{0x13, 0x1a, 0x22, 0xee})),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(16)),
			widget.RowLayoutOpts.Spacing(12),
		)),
	)

	// --- ヘッダ ---
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("GoTuber Tweaks", facePtr, color.White),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	))

	// --- Mouse Responsiveness ラベル ---
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("Mouse Follow", facePtr, labelColorIdle),
	))

	// --- Mouse Responsiveness スライダー (0-100 スケール) ---
	initialResp := int(state.MouseResponsiveness * 100)
	if initialResp < 5 {
		initialResp = 5
	}
	if initialResp > 100 {
		initialResp = 100
	}
	slider := widget.NewSlider(
		widget.SliderOpts.Orientation(widget.DirectionHorizontal),
		widget.SliderOpts.MinMax(5, 100),
		widget.SliderOpts.InitialCurrent(initialResp),
		widget.SliderOpts.WidgetOpts(widget.WidgetOpts.MinSize(200, 16)),
		widget.SliderOpts.Images(
			&widget.SliderTrackImage{
				Idle:  image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
				Hover: image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
			},
			&widget.ButtonImage{
				Idle:    image.NewNineSliceColor(color.NRGBA{0x66, 0x8a, 0xbf, 0xff}),
				Hover:   image.NewNineSliceColor(color.NRGBA{0x77, 0x9a, 0xcf, 0xff}),
				Pressed: image.NewNineSliceColor(color.NRGBA{0x55, 0x7a, 0xaf, 0xff}),
			},
		),
		widget.SliderOpts.FixedHandleSize(12),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			state.MouseResponsiveness = float64(args.Current) / 100.0
		}),
	)
	root.AddChild(slider)

	// --- Auto Blink トグル ---
	initialBlink := widget.WidgetUnchecked
	if state.BlinkEnabled {
		initialBlink = widget.WidgetChecked
	}
	blinkCheck := widget.NewCheckbox(
		widget.CheckboxOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionStart,
		})),
		widget.CheckboxOpts.Image(loadCheckboxImage()),
		widget.CheckboxOpts.InitialState(initialBlink),
		widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
			state.BlinkEnabled = args.State == widget.WidgetChecked
		}),
	)
	// checkbox + label を Row で並べる
	blinkRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
		)),
	)
	blinkRow.AddChild(blinkCheck)
	blinkRow.AddChild(widget.NewText(
		widget.TextOpts.Text("Auto Blink", facePtr, labelColorIdle),
	))
	root.AddChild(blinkRow)

	// --- Mic Mouth Movement トグル ---
	initialAudio := widget.WidgetUnchecked
	if state.AudioEnabled {
		initialAudio = widget.WidgetChecked
	}
	if !audioEnabled {
		// マイク利用不可 → チェックボックスを無効化 (greyed 表示)
		initialAudio = widget.WidgetGreyed
	}
	audioCheck := widget.NewCheckbox(
		widget.CheckboxOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionStart,
		})),
		widget.CheckboxOpts.Image(loadCheckboxImage()),
		widget.CheckboxOpts.InitialState(initialAudio),
		widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
			// マイク無効時は変更を無視
			if !audioEnabled {
				return
			}
			state.AudioEnabled = args.State == widget.WidgetChecked
		}),
	)
	audioRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
		)),
	)
	audioRow.AddChild(audioCheck)
	audioRow.AddChild(widget.NewText(
		widget.TextOpts.Text("Mic Mouth Movement", facePtr, labelColorIdle),
	))
	root.AddChild(audioRow)

	// --- ヒント ---
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("F1: Toggle Panel  |  Esc / Q / Ctrl+C: Quit", facePtr, labelColorDim),
	))

	// --- Quit ボタン ---
	quitBtn := widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(color.NRGBA{0x66, 0x3a, 0x3a, 0xff}),
			Hover:   image.NewNineSliceColor(color.NRGBA{0x88, 0x4a, 0x4a, 0xff}),
			Pressed: image.NewNineSliceColor(color.NRGBA{0x55, 0x2a, 0x2a, 0xff}),
		}),
		widget.ButtonOpts.Text("Quit", facePtr, btnTextColor),
		widget.ButtonOpts.TextPadding(&widget.Insets{Left: 20, Right: 20, Top: 5, Bottom: 5}),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			// ボタンクリックで終了。ebiten.Termination を返す手段がないため、
			// killswitch を直接トリガーする。
			state.QuitRequested = true
		}),
	)
	root.AddChild(quitBtn)

	p.ui = &ebitenui.UI{Container: root}
	return p
}

// Update は毎フレーム呼ばれる。
func (p *Panel) Update() {
	p.ui.Update()
}

// Draw は panel を screen に描画する。
func (p *Panel) Draw(screen *ebiten.Image) {
	p.ui.Draw(screen)
}

// loadCheckboxImage はシンプルなチェックボックス画像を作成する。
func loadCheckboxImage() *widget.CheckboxImage {
	return &widget.CheckboxImage{
		Unchecked:         image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
		Checked:           image.NewNineSliceColor(color.NRGBA{0x66, 0x8a, 0xbf, 0xff}),
		Greyed:            image.NewNineSliceColor(color.NRGBA{0x44, 0x44, 0x44, 0xff}),
		UncheckedHovered:  image.NewNineSliceColor(color.NRGBA{0x44, 0x4a, 0x54, 0xff}),
		CheckedHovered:    image.NewNineSliceColor(color.NRGBA{0x77, 0x9a, 0xcf, 0xff}),
		GreyedHovered:     image.NewNineSliceColor(color.NRGBA{0x55, 0x55, 0x55, 0xff}),
		UncheckedDisabled: image.NewNineSliceColor(color.NRGBA{0x22, 0x28, 0x32, 0xff}),
		CheckedDisabled:   image.NewNineSliceColor(color.NRGBA{0x44, 0x55, 0x66, 0xff}),
		GreyedDisabled:    image.NewNineSliceColor(color.NRGBA{0x33, 0x33, 0x33, 0xff}),
	}
}
