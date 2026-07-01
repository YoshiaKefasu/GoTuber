package tweaks

import (
	"fmt"
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

// Mic sensitivity スライダーの値域 (int、表示用)。内部表現は float 1.0..20.0x
// で保持し、スライダー値 = int / 10 で換算する (0.1 刻み、20.0x = 200)。
//
// Phase 1.14.15: 実機の無音ノイズでも口パクしないように感度を slider で調整可能化。
// default 10.0x、range 1.0..20.0x (audio.Mover.SetSensitivity のクランプ範囲と一致)。
const (
	sensitivitySliderMin = 10  // 1.0x
	sensitivitySliderMax = 200 // 20.0x
)

// Phase 4.3 hotfix: Morph Strength スライダーの値域 (int、表示用)。
// 内部表現 MorphStrength は 0.0..8.0 で保持し、スライダー値そのまま int。
// 0 = morph 無効相当、4 = デフォルト、8 = 最大。
const (
	morphStrengthSliderMin = 0
	morphStrengthSliderMax = 8
)

// Phase 4.3: Transition Duration スライダーの値域 (int ms、表示用)。
// 内部表現 TransitionDuration は ms で保持し、game.go で /1000 して秒に変換。
// 50..400ms。250ms が Phase 4.5 tuning デフォルト。
const (
	transitionDurationSliderMin = 50
	transitionDurationSliderMax = 400
)

// Phase 2.8.1: game.CameraModeCamera と同じ値。
// tweaks は game から import されるため、panel.go から game を import すると循環する。
const panelCameraModeCamera = 1

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
	micContainer        *widget.Container // ComboBox + Refresh を入れる Row コンテナ
	micCombo            *widget.ListComboButton
	refreshBtn          *widget.Button
	audioDebugText      *widget.Text // 1 行目: raw RMS / Floor / Gate (Phase 1.14.14)
	audioDebugText2     *widget.Text // 2 行目: Gated / Envelope / Mouth (Phase 1.14.14)
	micSensitivityLabel *widget.Text // "Mic Sensitivity: 10.0x" 動的ラベル (Phase 1.14.15)
	onDeviceSelected    func(deviceID string)
	onRefreshDevices    func() []audio.Device

	// Phase 1.14.16: Save ボタン + statusLabel 用フィールド。
	// Dirty 化 (ChangedHandler 側) と Save 成功/失敗 status 表示。
	// Reset 機能は YAGNI 削除 (Phase 1.14.16 Round 3): ebitenui slider の
	// 内部 Render() が lastCurrent != Current で再 fire する問題を回避するため、
	// RefreshWidgetsFromState() / mutingSliders / Checkbox 同期を全部捨てた。
	saveBtn         *widget.Button
	statusLabel     *widget.Text
	statusMessage   string // SetStatus 経由での上書き、毎フレーム参照される
	onSaveRequested func() error

	// Phase 2.8: Camera Status 表示 + Manual Restart ボタン。
	cameraStatusText         *widget.Text
	cameraRestartBtn         *widget.Button
	onCameraRestartRequested func()

	// Phase 2.10.8: Camera Enabled トグル (checkbox 本体への参照)。
	cameraCheck *widget.Checkbox

	// Phase 4.3: Morph Strength 動的ラベル ("Morph Strength: 8.0px")
	morphStrengthLabel *widget.Text

	// Phase 1.14.16: 起動時 ComboBox 初期選択同期用。
	// main.go が NewPanel に渡した initialDeviceID を保持。
	// SetDevices() の ComboBox 作成直後に selectDeviceByID() で適用。
	initialDeviceID string

	// Phase 1.14.16: ComboBox エントリを保持。SetDevices() で ComboBox 再作成後に
	// selectDeviceByID() で初期選択するための検索対象。
	currentEntries []any
}

// NewPanel は ebitenui を使った Tweaks パネルを構築する。
// face: ロード済み *text.GoTextFace (Gen Interface JP Regular)
// state: パネルの状態を保持する State
// audioEnabled: マイクが利用可能な場合 true。false のとき Audio checkbox は操作不可、
//
//	ComboBox は "(unavailable)" 表示、Refresh は動く (OS に再列挙要求は有効)。
//
// initialDeviceID: Phase 1.14.16 で追加。TOML から復元した malgo デバイス ID。
//
//	空文字 = OS default。ComboBox 起動時選択をこれに同期する。
func NewPanel(face *text.GoTextFace, state *State, audioEnabled bool, initialDeviceID string) *Panel {
	p := &Panel{
		state:           state,
		face:            face,
		initialDeviceID: initialDeviceID, // Phase 1.14.16: ComboBox 初期選択同期用
	}
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
			// Phase 1.14.16: スライダー変更は Dirty 化のみ。TOML 書込みは Save ボタン待ち。
			state.Dirty = true
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
			// Phase 1.14.16: チェックボックス変更は Dirty 化のみ。
			state.Dirty = true
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
		// Phase 1.14.16 (Critical #1 fix): tri-state 有効化が必須。
		// ebitenui v0.7.3 checkbox.go:116-118 で
		//   "non-tri state Checkbox cannot be in greyed state"
		//   panic を回避するため TriState() を必ず付ける。
		initialAudio = widget.WidgetGreyed
	}
	audioCheck := widget.NewCheckbox(
		widget.CheckboxOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionStart,
		})),
		widget.CheckboxOpts.Image(loadCheckboxImage()),
		widget.CheckboxOpts.TriState(), // Phase 1.14.16: WidgetGreyed を使うため必須
		widget.CheckboxOpts.InitialState(initialAudio),
		widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
			// マイク無効時は変更を無視
			if !audioEnabled {
				return
			}
			state.AudioEnabled = args.State == widget.WidgetChecked
			// Phase 1.14.16: チェックボックス変更は Dirty 化のみ。
			state.Dirty = true
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

	// --- Mic Sensitivity slider (Phase 1.14.15) ---
	// 実機無音ノイズでも口パクする問題の調整用。1.0x..20.0x (UI 10..200, 内部 /10.0)。
	// label は動的に "Mic Sensitivity: 10.0x" を表示し、Update() で state から再描画。
	micSensitivityLabel := widget.NewText(
		widget.TextOpts.Text(micSensitivityLabelText(state), facePtr, labelColorIdle),
	)
	root.AddChild(micSensitivityLabel)
	p.micSensitivityLabel = micSensitivityLabel

	initialSens := clampInt(int(state.AudioSensitivity*10.0), sensitivitySliderMin, sensitivitySliderMax)
	sensitivitySlider := widget.NewSlider(
		widget.SliderOpts.Orientation(widget.DirectionHorizontal),
		widget.SliderOpts.MinMax(sensitivitySliderMin, sensitivitySliderMax),
		widget.SliderOpts.InitialCurrent(initialSens),
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
			// int 10..200 → float 1.0..20.0x。0.1 刻み。
			state.AudioSensitivity = float64(args.Current) / 10.0
			// Phase 1.14.16: スライダー変更は Dirty 化のみ。
			state.Dirty = true
		}),
	)
	root.AddChild(sensitivitySlider)

	// --- Audio debug values ---
	// Phase 1.14.13: 口パクしない問題の切り分け用。RMS=0 なら入力が来ていない、
	// RMS は動くが Envelope/Mouth が動かないなら閾値側、Mouth が動くなら描画側を疑う。
	// Phase 1.14.14: 2 行に拡張 (1 行目: raw + floor + gate / 2 行目: gated + envelope + mouth)
	// 固定閾値チューニングと adaptive noise gate の状態をユーザーが観察できるように。
	audioDebugText := widget.NewText(
		widget.TextOpts.Text(audioDebugLabel1(state), facePtr, labelColorDim),
	)
	root.AddChild(audioDebugText)
	p.audioDebugText = audioDebugText

	audioDebugText2 := widget.NewText(
		widget.TextOpts.Text(audioDebugLabel2(state), facePtr, labelColorDim),
	)
	root.AddChild(audioDebugText2)
	p.audioDebugText2 = audioDebugText2

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

	// --- Phase 1.14.16: Save ボタン + statusLabel ---
	// 起動直後は Dirty=false なので disable。ChangedHandler で Dirty=true になると enable。
	// Reset ボタンは Round 3 で YAGNI 削除。
	saveBtn := widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(color.NRGBA{0x44, 0x66, 0x44, 0xff}),
			Hover:   image.NewNineSliceColor(color.NRGBA{0x55, 0x77, 0x55, 0xff}),
			Pressed: image.NewNineSliceColor(color.NRGBA{0x33, 0x55, 0x33, 0xff}),
		}),
		widget.ButtonOpts.Text("Save", facePtr, btnTextColor),
		widget.ButtonOpts.TextPadding(&widget.Insets{Left: 20, Right: 20, Top: 5, Bottom: 5}),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if !state.Dirty {
				return
			}
			if p.onSaveRequested == nil {
				return
			}
			// Save 処理 (TOML 書込み) は main.go 側でオーケストレート。
			// エラー → statusLabel に "save failed: ..."、成功 → "saved" + Dirty=false。
			if err := p.onSaveRequested(); err != nil {
				p.SetStatus("save failed: " + err.Error())
				return
			}
			state.Dirty = false
			p.SetStatus("saved")
		}),
	)
	saveBtn.GetWidget().Disabled = true // 起動直後は Dirty=false
	p.saveBtn = saveBtn

	// Phase 1.14.16 Round 3: Reset ボタンは YAGNI 削除。
	// ebitenui slider の Render() が lastCurrent != Current で再 fire する
	// 構造的問題があり、RefreshWidgetsFromState() 系の実装が根本的に難しい。
	// 詳細は code-reviewer Round 2 REJECT フィードバック参照。
	// 代替: dirty な変更を破棄したい場合は TOML を直接削除するか、
	// 起動時の「設定確認あればロードするとなければデフォルトにします」動作を利用。
	root.AddChild(saveBtn)

	statusLabel := widget.NewText(
		widget.TextOpts.Text(statusText(state), facePtr, labelColorDim),
	)
	root.AddChild(statusLabel)
	p.statusLabel = statusLabel

	// --- Phase 2.10.8: Camera Enabled トグル ---
	initialCamera := widget.WidgetUnchecked
	if state.CameraEnabled {
		initialCamera = widget.WidgetChecked
	}
	cameraCheck := widget.NewCheckbox(
		widget.CheckboxOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionStart,
			}),
			// Phase 2.10.8 fix: NewNineSliceColor は MinSize (0,0) を返すため、
			// 明示的な MinSize を指定しないと checkbox が 0x0 で描画され invisible になる。
			widget.WidgetOpts.MinSize(16, 16),
		),
		widget.CheckboxOpts.Image(loadCheckboxImage()),
		widget.CheckboxOpts.InitialState(initialCamera),
		widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
			state.CameraEnabled = args.State == widget.WidgetChecked
			state.Dirty = true
		}),
	)
	cameraRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
		)),
	)
	cameraRow.AddChild(cameraCheck)
	cameraRow.AddChild(widget.NewText(
		widget.TextOpts.Text("Camera Enabled", facePtr, labelColorIdle),
	))
	p.cameraCheck = cameraCheck
	root.AddChild(cameraRow)

	// --- Phase 2.8: Camera Status + Manual Restart ---
	root.AddChild(p.NewCameraSection(nil))

	// --- Phase 4.3: Morph Renderer セクション ---
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("Morph Renderer", facePtr, labelColorIdle),
	))

	// Morph ON/OFF トグル
	initialMorph := widget.WidgetUnchecked
	if state.MorphEnabled {
		initialMorph = widget.WidgetChecked
	}
	morphCheck := widget.NewCheckbox(
		widget.CheckboxOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionStart,
		})),
		widget.CheckboxOpts.WidgetOpts(
			// Phase 2.10.8 camera checkbox と同じ理由。
			// NewNineSliceColor ベースの checkbox image は MinSize 未指定だと 0x0 になり、
			// ラベルだけ見えてトグル本体が invisible になる。
			widget.WidgetOpts.MinSize(16, 16),
		),
		widget.CheckboxOpts.Image(loadCheckboxImage()),
		widget.CheckboxOpts.InitialState(initialMorph),
		widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
			state.MorphEnabled = args.State == widget.WidgetChecked
			state.Dirty = true
		}),
	)
	morphRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
		)),
	)
	morphRow.AddChild(morphCheck)
	morphRow.AddChild(widget.NewText(
		widget.TextOpts.Text("Depth Morph", facePtr, labelColorIdle),
	))
	root.AddChild(morphRow)

	// Morph Strength スライダー (0..16 px)
	morphStrengthLabelText := widget.NewText(
		widget.TextOpts.Text(morphStrengthLabelFmt(state), facePtr, labelColorIdle),
	)
	root.AddChild(morphStrengthLabelText)
	p.morphStrengthLabel = morphStrengthLabelText

	initialMorphStr := clampInt(int(state.MorphStrength), morphStrengthSliderMin, morphStrengthSliderMax)
	morphStrSlider := widget.NewSlider(
		widget.SliderOpts.Orientation(widget.DirectionHorizontal),
		widget.SliderOpts.MinMax(morphStrengthSliderMin, morphStrengthSliderMax),
		widget.SliderOpts.InitialCurrent(initialMorphStr),
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
			state.MorphStrength = float64(args.Current)
			state.Dirty = true
		}),
	)
	root.AddChild(morphStrSlider)

	// Transition Duration スライダー (50..200 ms)
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("Transition (ms)", facePtr, labelColorIdle),
	))

	initialTransDur := clampInt(int(state.TransitionDuration), transitionDurationSliderMin, transitionDurationSliderMax)
	transDurSlider := widget.NewSlider(
		widget.SliderOpts.Orientation(widget.DirectionHorizontal),
		widget.SliderOpts.MinMax(transitionDurationSliderMin, transitionDurationSliderMax),
		widget.SliderOpts.InitialCurrent(initialTransDur),
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
			state.TransitionDuration = float64(args.Current)
			state.Dirty = true
		}),
	)
	root.AddChild(transDurSlider)

	// --- ヒント ---
	root.AddChild(widget.NewText(
		widget.TextOpts.Text("F1: Toggle Panel  |  Ctrl+Shift+H: Hide All UI", facePtr, labelColorDim),
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
			// ボタンクリックで終了。Game.Update() で QuitRequested チェック →
			// ebiten.Termination 返却 (Phase 1.14 後の唯一の GUI 終了手段。
			// Esc / signal.Notify on Windows は削除済み)
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
				// Phase 1.14.11 fix: Mask 未設定だと ebitenui v0.7.3 の
				// ScrollContainer.Validate() で panic する (Refresh ボタン押下時の
				// ComboBox 再生成で発火)。Idle と同じ色でクリッピング形状を定義。
				Mask: image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
			},
			Slider: &widget.SliderParams{
				TrackImage: &widget.SliderTrackImage{
					Idle:  image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
					Hover: image.NewNineSliceColor(color.NRGBA{0x44, 0x4a, 0x54, 0xff}),
				},
				// Phase 1.14.12 fix: List の縦 Slider handle は Button として描画される。
				// HandleImage 未設定だと ebitenui v0.7.3 の Button.draw() で nil panic する。
				HandleImage: &widget.ButtonImage{
					Idle:    image.NewNineSliceColor(color.NRGBA{0x5b, 0x66, 0x73, 0xff}),
					Hover:   image.NewNineSliceColor(color.NRGBA{0x72, 0x7d, 0x8a, 0xff}),
					Pressed: image.NewNineSliceColor(color.NRGBA{0x47, 0x52, 0x5f, 0xff}),
				},
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

// NewCameraSection は Camera Status 表示 + Restart ボタンのミニセクションを返す。
//
// Phase 2.8: F1 panel の下部に追加する表示専用セクション。Restart ボタンは
// Down 状態時のみ有効化され、押下時は onRestartRequested または setter で登録された
// callback に処理を委譲する。
func (p *Panel) NewCameraSection(onRestartRequested func()) *widget.Container {
	if onRestartRequested != nil {
		p.onCameraRestartRequested = onRestartRequested
	}
	faceIface := text.Face(p.face)
	facePtr := &faceIface
	labelColorIdle := color.NRGBA{0xcc, 0xcc, 0xcc, 0xff}
	btnTextColor := &widget.ButtonTextColor{
		Idle:     color.NRGBA{0xdf, 0xf4, 0xff, 0xff},
		Disabled: color.NRGBA{0x99, 0x99, 0x99, 0xff},
	}

	p.cameraStatusText = widget.NewText(
		widget.TextOpts.Text("Camera: Mouse", facePtr, labelColorIdle),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	p.cameraRestartBtn = widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(color.NRGBA{0x33, 0x3a, 0x44, 0xff}),
			Hover:   image.NewNineSliceColor(color.NRGBA{0x44, 0x4a, 0x54, 0xff}),
			Pressed: image.NewNineSliceColor(color.NRGBA{0x22, 0x28, 0x32, 0xff}),
		}),
		widget.ButtonOpts.Text("Restart Camera", facePtr, btnTextColor),
		widget.ButtonOpts.TextPadding(&widget.Insets{Left: 12, Right: 12, Top: 5, Bottom: 5}),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if p.onCameraRestartRequested != nil {
				p.onCameraRestartRequested()
			}
		}),
	)
	p.cameraRestartBtn.GetWidget().Disabled = true

	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(4)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	section.AddChild(p.cameraStatusText)
	section.AddChild(p.cameraRestartBtn)
	return section
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

// SetOnSaveRequested は Save ボタン押下時のコールバックを設定する (Phase 1.14.16)。
// main.go 側で config.Save + state.Dirty=false + panel.SetStatus("saved") を実行する。
// エラー時は statusLabel を "save failed: ..." に更新した上で err を返す。
func (p *Panel) SetOnSaveRequested(fn func() error) {
	p.onSaveRequested = fn
}

// SetOnCameraRestartRequested は Restart Camera ボタン押下時の callback を設定する。
//
// Phase 2.8: mp_server.py の実制御は supervisor に委譲し、Panel は要求通知だけを行う。
func (p *Panel) SetOnCameraRestartRequested(fn func()) {
	p.onCameraRestartRequested = fn
}

// SetStatus は statusLabel の文字列を更新する (Phase 1.14.16)。
// 公開 setter 経由で更新することで Update() の毎フレーム再代入と競合しない。
func (p *Panel) SetStatus(message string) {
	p.statusMessage = message
	if p.statusLabel != nil {
		p.statusLabel.Label = message
	}
}

// UpdateCameraStatus は Camera Status 表示と Restart ボタン状態を更新する。
//
// Phase 2.8: Game.Update() から毎フレーム呼ばれる。mode は 0=Mouse / 1=Camera。
// lastErr が nil の場合はエラーなし、非 nil の場合は mp_server.py または detection 系の
// 異常表示に使う。TOML 永続化は行わない表示専用 state。
//
// Phase 2.10.8: cameraEnabled パラメータを追加。false のとき Restart ボタンを
// disabled にし、"Camera: Disabled" 表示にする。
func (p *Panel) UpdateCameraStatus(mode int, mpRunning bool, lastErr *string, cameraEnabled bool) {
	restartable := true // ユーザー希望: 全状態で Restart ボタンを常時クリック可能にする。

	if !cameraEnabled {
		// Phase 2.10.8: Camera Enabled OFF → ボタン無効 + "Disabled" 表示
		if p.cameraStatusText != nil {
			p.cameraStatusText.Label = "Camera: Disabled"
		}
		if p.cameraRestartBtn != nil {
			p.cameraRestartBtn.GetWidget().Disabled = true
		}
		p.state.CameraMode = "Disabled"
		p.state.CameraRestartable = false
		return
	}

	status := "Mouse"

	switch {
	case !mpRunning:
		status = "Down"
	case mpRunning && lastErr != nil:
		status = "Lost Signal"
	case mode == panelCameraModeCamera:
		status = "Active"
	default:
		status = "Mouse"
	}

	if p.cameraStatusText != nil {
		p.cameraStatusText.Label = "Camera: " + status
	}
	if p.cameraRestartBtn != nil {
		p.cameraRestartBtn.GetWidget().Disabled = !restartable
	}
	p.state.CameraMode = status
	p.state.CameraRestartable = restartable
}

// statusText は statusLabel に表示する文字列を返す (Phase 1.14.16)。
// state.Dirty が true なら "unsaved changes"、それ以外なら空文字。
//
// 優先順位ロジックは Update() (panel.go:624 周辺) に集約されており、
// statusMessage (SetStatus で保存された Save/エラー通知) を state.Dirty より優先表示する。
// この関数自体は "unsaved changes" 表示の単機能。
func statusText(state *State) string {
	// 呼び出し側が p.statusMessage を保持しているので、ここでは state のみ参照。
	if state.Dirty {
		return "unsaved changes"
	}
	return ""
}

// SetDevices は ComboBox のエントリを新しいデバイス一覧で置き換える。
// 呼び出しスレッド: ebitenui メインスレッドから (Refresh ボタン or Game.Update 内
// dispatch)。ebitenui は goroutine safe ではないため、バックグラウンド goroutine
// から直接呼ぶのは禁止。
//
// Phase 1.13a 起動時: main.go の goroutine → devicesCh → Game.Update() 経由で
//
//	ebitenui メインスレッドから呼ばれる (S-1: channel dispatch)。
//
// Phase 1.13a Refresh: Refresh ボタンの ClickedHandler 内 (ebitenui メインスレッド)。
//
// devices に空配列を渡すと ComboBox は "(OS default)" のみになる。
//
// Phase 1.14.16: パネルに保存された initialDeviceID にマッチするエントリがあれば
// ComboBox の初期選択をそれに同期する。TOML から復元したデバイス名で起動時に
// 視覚確認できる。空文字 (= OS default) の場合は "(OS default)" エントリを選択。
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
	p.currentEntries = entries // Phase 1.14.16: 初期選択用に保持
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

	// Phase 1.14.16: 初期 deviceID にマッチするエントリを選択。
	// 起動時に TOML から復元した device_id を視覚確認できるようにする。
	p.selectDeviceByID(p.initialDeviceID)
}

// selectDeviceByID は ComboBox のエントリから ID が一致するものを選択する (Phase 1.14.16)。
// 空文字なら先頭の "(OS default)" を選択。一致なしなら何もしない。
//
// 注意: SetSelectedEntry() は内部で EntrySelectedEvent を発火するため、
// main.go の SetOnDeviceSelected コールバック (Phase 1.13a / 1.14.7 で実装済) が
// 呼ばれる。起動時 ComboBox 初期選択時、main.go の guard (currentDeviceID との
// 一致チェック) で no-op になるので、Restart も Save も走らない。
func (p *Panel) selectDeviceByID(deviceID string) {
	if p.micCombo == nil || len(p.currentEntries) == 0 {
		return
	}
	for _, e := range p.currentEntries {
		if d, ok := e.(audio.Device); ok && d.ID == deviceID {
			p.micCombo.SetSelectedEntry(e)
			return
		}
	}
	// 一致なし: 先頭 (OS default) を選択
	p.micCombo.SetSelectedEntry(p.currentEntries[0])
}

// Update は毎フレーム呼ばれる。
func (p *Panel) Update() {
	if p.audioDebugText != nil {
		p.audioDebugText.Label = audioDebugLabel1(p.state)
	}
	if p.audioDebugText2 != nil {
		p.audioDebugText2.Label = audioDebugLabel2(p.state)
	}
	if p.micSensitivityLabel != nil {
		// Phase 1.14.15: スライダー操作中の現在値を毎フレーム反映。
		// スライダーの ChangedHandler は state.AudioSensitivity を更新するので、
		// ここでは state から読んで Label に再代入する。
		p.micSensitivityLabel.Label = micSensitivityLabelText(p.state)
	}
	if p.morphStrengthLabel != nil {
		// Phase 4.3: Morph Strength スライダー操作中の現在値を毎フレーム反映。
		p.morphStrengthLabel.Label = morphStrengthLabelFmt(p.state)
	}
	if p.statusLabel != nil {
		// Phase 1.14.16: status 表示更新。
		// SetStatus で上書きされた statusMessage があればそれ、なければ state.Dirty から判定。
		if p.statusMessage != "" {
			p.statusLabel.Label = p.statusMessage
		} else {
			p.statusLabel.Label = statusText(p.state)
		}
	}
	// Phase 1.14.16: Save ボタンの enable/disable を Dirty で切替。
	// Disable 状態だとクリックイベントが発火しない (ebitenui widget convention)。
	if p.saveBtn != nil {
		p.saveBtn.GetWidget().Disabled = !p.state.Dirty
	}
	p.ui.Update()
}

// audioDebugLabel1 は 1 行目: 入力側 (raw RMS / noise floor / gate 状態)。
// 例: "Audio RMS: 0.0038 | Floor: 0.0012 | Gate: open"
//
// 読み方:
//   - RMS=0 → マイク入力が来ていない (device 不正 or ミュート)
//   - RMS>0, Floor も同程度, Gate closed → 環境ノイズのみ (正常)
//   - RMS が Floor + 0.008 を超えれば Gate open になりやすい
//   - Gate open なのに Mouth が動かない → gain / envelope 閾値側を疑う
func audioDebugLabel1(state *State) string {
	return fmt.Sprintf("Audio RMS: %.4f | Floor: %.4f | Gate: %s",
		state.AudioRMS,
		state.AudioNoiseFloor,
		gateStateLabel(state.AudioGateOpen),
	)
}

// audioDebugLabel2 は 2 行目: 処理結果 (gated / envelope / mouth)。
// 例: "Gated: 0.0420 | Envelope: 0.0310 | Mouth: closed"
//
// 読み方:
//   - Gated=0 → Gate closed (gate で切られている、env/mouth も 0)
//   - Gated>0, Envelope=0 → 直前で gate が閉じた (release 待ち)
//   - Envelope>0, Mouth=closed → 0.07 未満 (MouthHalf 閾値以下)
//   - Envelope>0.22, Mouth=open → 口全開
func audioDebugLabel2(state *State) string {
	return fmt.Sprintf("Gated: %.4f | Envelope: %.4f | Mouth: %s",
		state.AudioGatedRMS,
		state.AudioEnvelope,
		mouthStateLabel(state.AudioMouthState),
	)
}

func mouthStateLabel(mouthState int) string {
	switch mouthState {
	case audio.MouthHalf:
		return "half"
	case audio.MouthOpen:
		return "open"
	default:
		return "closed"
	}
}

func gateStateLabel(gateOpen bool) string {
	if gateOpen {
		return "open"
	}
	return "closed"
}

// micSensitivityLabelText は "Mic Sensitivity: 10.0x" 形式の動的ラベル文字列。
// Phase 1.14.15: スライダー値を 0.1 刻みの 1 桁小数で表示。min 1.0x / max 20.0x。
// state.AudioSensitivity は 0.1 刻みで 10..200 → 1.0..20.0x の値域。
func micSensitivityLabelText(state *State) string {
	return fmt.Sprintf("Mic Sensitivity: %.1fx", state.AudioSensitivity)
}

// morphStrengthLabelFmt は "Morph Strength: 8.0px" 形式の動的ラベル文字列。
// Phase 4.3: スライダー値を 1 桁小数で表示。min 0.0 / max 16.0。
func morphStrengthLabelFmt(state *State) string {
	return fmt.Sprintf("Morph Strength: %.1fpx", state.MorphStrength)
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
