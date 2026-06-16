package tweaks

import (
	stdimage "image"
	"image/color"

	"github.com/YoshiaKefasu/GoTuber/internal/audio"
	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Mouse follow スライダーの値域 (int、表示用)。
// 内部表現 MouseResponsiveness は 0.0-1.0 で保持し、スライダー値 = int * 100 で換算する。
const (
	sliderMin = 5   // 0.05 * 100
	sliderMax = 100 // 1.0 * 100
)

// clampInt は v を [lo, hi] にクランプする。
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Panel は Tweaks パネルの ebitenui UI 実装。
type Panel struct {
	ui    *ebitenui.UI
	state *State
	face  *text.GoTextFace // ComboBox ヘルパー (newDeviceCombo) で再利用するために保持

	// uiHidden: true のとき全 UI を非表示にする。
	// Phase 1.13b: Ctrl+Shift+H で toggle。Game.Update() から SetUIHidden() 経由で
	// 設定される。OBS ウィンドウキャプチャに Tweaks パネルが映り込まないようにする用途。
	uiHidden bool

	// Phase 1.13a: マイクデバイス選択 UI (ComboBox + Refresh ボタン) 用フィールド。
	// コールバックは main.go から setter で設定する (tweaks パッケージは audio 操作を直接行わない)。
	micContainer     *widget.Container // ComboBox + Refresh を入れる Row コンテナ
	micCombo         *widget.ListComboButton
	refreshBtn       *widget.Button
	onDeviceSelected func(deviceID string)
	onRefreshDevices func() []audio.Device
}

// NewPanel は ebitenui を使った Tweaks パネルを構築する。
// face: ロード済み *text.GoTextFace (Gen Interface JP Regular)
// state: パネルの状態を保持する State
// audioEnabled: マイクが利用可能な場合 true。false のとき Audio checkbox は操作不可、
//
//	ComboBox は "(unavailable)" 表示、Refresh は動く (OS に再列挙要求は有効)。
func NewPanel(face *text.GoTextFace, state *State, audioEnabled bool) *Panel {
	p := &Panel{state: state, face: face}
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

	// --- Mouse Responsiveness スライダー (int 0-100 スケール、内部で /100.0) ---
	initialResp := clampInt(int(state.MouseResponsiveness*100), sliderMin, sliderMax)
	slider := widget.NewSlider(
		widget.SliderOpts.Orientation(widget.DirectionHorizontal),
		widget.SliderOpts.MinMax(sliderMin, sliderMax),
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

	// --- Phase 1.13a: Microphone Device (ComboBox) + Refresh ボタン ---
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("Microphone Device", facePtr, labelColorIdle),
	))

	// ComboBox と Refresh を横並びで配置する Row コンテナ。
	// SetDevices() で ComboBox だけ Replace する (Refresh は使い回し)。
	micContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
		)),
	)

	// 初期エントリ: "OS default" 1 個 (空 ID)。
	// 起動時バックグラウンド列挙で main.go から panel.SetDevices() が呼ばれ、
	// ここに動的デバイス一覧が流し込まれる (Refresh 押下でも同様)。
	initialEntries := []any{
		audio.Device{ID: "", Name: "(OS default)"},
	}
	if !audioEnabled {
		// マイク利用不可 (audio 初期化失敗) → 1 個だけ「(unavailable)」表示
		initialEntries = []any{
			audio.Device{ID: "unavailable", Name: "(unavailable)"},
		}
	}

	combo := p.newDeviceCombo(initialEntries)
	micContainer.AddChild(combo)

	refreshBtn := widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
			Hover:   image.NewNineSliceColor(color.NRGBA{0x44, 0x4a, 0x54, 0xff}),
			Pressed: image.NewNineSliceColor(color.NRGBA{0x22, 0x28, 0x32, 0xff}),
		}),
		widget.ButtonOpts.Text("Refresh", facePtr, btnTextColor),
		widget.ButtonOpts.TextPadding(&widget.Insets{Left: 12, Right: 12, Top: 5, Bottom: 5}),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if p.onRefreshDevices == nil {
				return
			}
			devices := p.onRefreshDevices()
			p.SetDevices(devices)
		}),
	)
	micContainer.AddChild(refreshBtn)

	root.AddChild(micContainer)

	// パネルインスタンスに参照を保持 (Refresh での ComboBox 差し替え + Setter から更新)
	p.micContainer = micContainer
	p.micCombo = combo
	p.refreshBtn = refreshBtn

	// --- ヒント ---
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("F1: Toggle Panel  |  Esc / Q / Ctrl+C: Quit  |  Ctrl+Shift+H: Hide All UI", facePtr, labelColorDim),
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

// newDeviceCombo は ComboBox (widget.ListComboButton) を作るヘルパー。
// NewPanel (初期) と SetDevices (Refresh) の両方から呼ばれる。
// entries: ComboBox の選択肢 (audio.Device のスライス)。
//
// 選択変更は p.onDeviceSelected コールバックに委譲する。
func (p *Panel) newDeviceCombo(entries []any) *widget.ListComboButton {
	faceIface := text.Face(p.face)
	btnTextColor := &widget.ButtonTextColor{
		Idle:     color.NRGBA{0xdf, 0xf4, 0xff, 0xff},
		Disabled: color.NRGBA{0x99, 0x99, 0x99, 0xff},
	}
	labelFunc := func(e any) string {
		if d, ok := e.(audio.Device); ok {
			return d.Name
		}
		return "?"
	}
	return widget.NewListComboButton(
		widget.ListComboButtonOpts.Entries(entries),
		widget.ListComboButtonOpts.MaxContentHeight(150),
		widget.ListComboButtonOpts.ButtonParams(&widget.ButtonParams{
			Image: &widget.ButtonImage{
				Idle:    image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
				Hover:   image.NewNineSliceColor(color.NRGBA{0x44, 0x4a, 0x54, 0xff}),
				Pressed: image.NewNineSliceColor(color.NRGBA{0x22, 0x28, 0x32, 0xff}),
			},
			TextPadding: widget.NewInsetsSimple(5),
			TextColor:   btnTextColor,
			TextFace:    &faceIface,
			MinSize:     &stdimage.Point{X: 200, Y: 0},
		}),
		widget.ListComboButtonOpts.ListParams(&widget.ListParams{
			ScrollContainerImage: &widget.ScrollContainerImage{
				Idle: image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
			},
			EntryFace: &faceIface,
			EntryColor: &widget.ListEntryColor{
				Selected:   color.NRGBA{0xdf, 0xf4, 0xff, 0xff},
				Unselected: color.NRGBA{0xcc, 0xcc, 0xcc, 0xff},
			},
		}),
		widget.ListComboButtonOpts.EntryLabelFunc(labelFunc, labelFunc),
		widget.ListComboButtonOpts.EntrySelectedHandler(func(args *widget.ListComboButtonEntrySelectedEventArgs) {
			d, ok := args.Entry.(audio.Device)
			if !ok {
				return
			}
			if p.onDeviceSelected != nil {
				p.onDeviceSelected(d.ID)
			}
		}),
	)
}

// SetUIHidden は uiHidden フラグを設定する。
// Game.Update() から Ctrl+Shift+H 検出時に呼ばれる。
// true にすると Draw() は no-op になる (Game.Draw() 側でも
// !g.uiHidden でガードする二重防御)。
func (p *Panel) SetUIHidden(v bool) {
	p.uiHidden = v
}

// SetOnDeviceSelected は ComboBox エントリ選択時のコールバックを設定する。
// deviceID: 選択された audio.Device.ID (空文字 = OS default)。
// main.go はここで config.Save + Mover.Restart を実行する。
func (p *Panel) SetOnDeviceSelected(fn func(deviceID string)) {
	p.onDeviceSelected = fn
}

// SetOnRefreshDevices は Refresh ボタン押下時のデバイス再列挙コールバックを設定する。
// 戻り値: 新しいデバイス一覧。エラー時は nil を返しても良い。
// main.go はここで audio.ListDevices() を呼ぶ。
func (p *Panel) SetOnRefreshDevices(fn func() []audio.Device) {
	p.onRefreshDevices = fn
}

// SetDevices は ComboBox のエントリを新しいデバイス一覧で置き換える。
// 呼び出しスレッド: ebitenui メインスレッドから (Refresh ボタン or 起動時バックグラウンド
// からの呼び出しは不可、ebitenui は goroutine safe ではない)。
//
// Phase 1.13a 起動時: main.go の `go func() { ... panel.SetDevices(devices) }()` 経由。
// Phase 1.13a Refresh: Refresh ボタンの ClickedHandler 内 (ebitenui メインスレッド)。
//
// devices に空配列を渡すと ComboBox は "(OS default)" のみになる。
func (p *Panel) SetDevices(devices []audio.Device) {
	if p.micContainer == nil {
		return
	}
	// 古い ComboBox を削除 (Refresh ボタンは残す)
	if p.micCombo != nil {
		p.micContainer.RemoveChild(p.micCombo)
	}
	// 新しいエントリ: 先頭に "(OS default)" (空 ID) を必ず置く
	entries := make([]any, 0, len(devices)+1)
	entries = append(entries, audio.Device{ID: "", Name: "(OS default)"})
	for _, d := range devices {
		entries = append(entries, d)
	}
	combo := p.newDeviceCombo(entries)
	// ComboBox は micContainer の先頭 (Refresh ボタンの前) に配置
	// ebitenui Container の AddChild は末尾追加なので、Refresh ボタンを
	// 一時的に RemoveChild してから ComboBox + Refresh の順で Add する。
	if p.refreshBtn != nil {
		p.micContainer.RemoveChild(p.refreshBtn)
	}
	p.micContainer.AddChild(combo)
	if p.refreshBtn != nil {
		p.micContainer.AddChild(p.refreshBtn)
	}
	p.micCombo = combo
}

// Update は毎フレーム呼ばれる。
func (p *Panel) Update() {
	p.ui.Update()
}

// Draw は panel を screen に描画する。
// uiHidden == true のとき、UI を何も描画しない (OBS キャプチャに映らない)。
func (p *Panel) Draw(screen *ebiten.Image) {
	if p.uiHidden {
		return
	}
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
