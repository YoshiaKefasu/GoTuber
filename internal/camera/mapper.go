// Package camera の Phase 2.4 マッパー: MediaPipe 検出結果 (yaw / pitch / EAR / 顔中心) を
// Phase 1.12 の 5x5 atlas セル座標 + 瞬き状態に写像する純粋関数群。
//
// 本ファイルは 副作用なし・IO なし・ロックなしの純粋関数のみで構成されているため
// `//go:build camera` を付けず、Phase 1 ビルド (`-tags` なし) でも常にコンパイルされる。
// capture.go (Phase 2.2) / mpclient.go (Phase 2.3) は `//go:build camera` で
// ガードされているため、同居しても Go のビルド制約上問題ない (タグ有無で
// ファイル集合が切り替わるだけで、両方含む集合も両方含まない集合も valid)。
//
// Phase 2.5 (main.go 統合 / supervisor) と Phase 2.6 (EAR ヒステリシス) で
// この API を呼び出す。
package camera

import "math"

// Cell は 5x5 atlas のセル座標 (Phase 1.12 規約)。
//
// Col=0 は左端、Col=4 は右端、Row=0 は上端、Row=4 は下端。顔を正面に向けた状態は
// Col=2, Row=2 (中央)。
//
// Phase 2.4: 現状は YawToCol / PitchToRow がそれぞれ int を返す設計だが、将来の
// supervisor (Phase 2.5) やアセット選択ロジックで col+row を同時に扱う用途を
// 見越して公開しておく (YAGNI 厳守のため Phase 2.4 では実使用なし)。
type Cell struct {
	Col int // 0=left, 2=center, 4=right
	Row int // 0=top,    2=center, 4=bottom
}

// BlinkState は瞬き状態。Phase 1.6 のまばたき scheduler が口の D-E-F セル切替に
// 使う (Phase 2.4 では判別のみ提供、駆動は Phase 2.5)。
//
// 整数ベースで `switch` 文に直接渡せるよう明示的な整数値を採番。
type BlinkState int

const (
	BlinkOpen   BlinkState = 0
	BlinkHalf   BlinkState = 1
	BlinkClosed BlinkState = 2
)

// 閾値定数 (Phase 2.4: 5x5 atlas セル算出用)。
//
// EAR は tools/mp_server.py の _compute_ear() が返す素の値 (典型域 ~0.05–0.35)。
// 0.22 / 0.10 は MediaPipe 出力の実測分布に基づく Phase 2.4 独自閾値で、
// Python 側に同値定数はない (Go 側のみで適用)。Phase 2.6 でヒステリシスを
// 別レイヤで被せる想定 (現状は単純な 2 段しきい値)。
//
// yaw/pitch 範囲 (±30°/±20°) は画面前で使う現実的な head pose 範囲。
// profile view (>±30°) では mapper が clamp で最大セルを返すため、
// camera follow モードでは mouse follow fallback を促す意図。
const (
	yawMinDeg   = -30.0
	yawMaxDeg   = +30.0
	pitchMinDeg = -20.0
	pitchMaxDeg = +20.0

	earOpenThreshold   = 0.22
	earClosedThreshold = 0.10

	// faceDetectionTimeoutSec は「最後に顔検出成功から現在時刻までの猶予秒」。
	// 1.0 秒 (= 60Hz で約 60 フレーム) を超えたら mouse.Follower にフォールバック
	// する。docs/PHASE2.md §1.1 (配信中可用性方針) と整合。
	faceDetectionTimeoutSec = 1.0
)

// YawToCol は yaw 角 (度) を 5x5 atlas の col インデックス (0..4) に変換する。
//
// マッピング:
//   - -30° → 0 (左端)
//   - 0° → 2 (中央)
//   - +30° → 4 (右端)
//   - 範囲外 (-30..+30 外) はクランプして端のセルを返す
//
// 戻り値は 0,1,2,3,4 のいずれか。誤差を含む入力でも math.Round で最近接整数に丸める。
//
// Phase 2.4: 純粋関数・副作用なし。Phase 2.5 の supervisor が毎フレーム呼び出す。
func YawToCol(yawDeg float64) int {
	if yawDeg < yawMinDeg {
		yawDeg = yawMinDeg
	}
	if yawDeg > yawMaxDeg {
		yawDeg = yawMaxDeg
	}
	colF := (yawDeg - yawMinDeg) / (yawMaxDeg - yawMinDeg) * 4.0
	return int(math.Round(colF))
}

// PitchToRow は pitch 角 (度) を 5x5 atlas の row インデックス (0..4) に変換する。
//
// Y軸反転: pitch +20° (上を向く) → row 0 (上端)。「上を向く = 画面上を見る」なので
// 画面座標系と一致する。pitch -20° (下) → row 4 (下端)。pitch 0° → row 2 (中央)。
//
// 範囲外 (-20..+20 外) はクランプして端のセルを返す。
//
// Phase 2.4: 純粋関数・副作用なし。YawToCol と対称。
func PitchToRow(pitchDeg float64) int {
	if pitchDeg < pitchMinDeg {
		pitchDeg = pitchMinDeg
	}
	if pitchDeg > pitchMaxDeg {
		pitchDeg = pitchMaxDeg
	}
	rowF := 4.0 - (pitchDeg-pitchMinDeg)/(pitchMaxDeg-pitchMinDeg)*4.0
	return int(math.Round(rowF))
}

// EARToBlink は左右両耳の EAR (Eye Aspect Ratio) 平均値から瞬き状態を判定する。
//
// 判定:
//   - 平均 EAR >= 0.22 → BlinkOpen  (開眼)
//   - 平均 EAR <= 0.10 → BlinkClosed (閉眼)
//   - 0.10 < 平均 EAR < 0.22 → BlinkHalf (半目)
//   - 範囲外 (< 0 or > 0.5) → BlinkOpen にフォールバック (MediaPipe の noise 対策)
//
// Phase 1.6 のまばたき scheduler は BlinkHalf を D-E の中間状態として使う想定だが、
// Phase 2.4 では 3 状態の判別のみ提供し、ヒステリシスや中間フレーム処理は
// Phase 2.6 側で担当する (YAGNI)。
//
// 引数は左右独立。左右非対称でも平均値で代表するため、片目だけ眩しいケースなど
// 異常値は平均で平滑化される。
//
// Phase 2.4: 純粋関数・副作用なし。
func EARToBlink(earLeft, earRight float64) BlinkState {
	earAvg := (earLeft + earRight) / 2.0
	// 不正値 (負値 / 0.5 超) は MediaPipe noise 由来 → 開眼側へフェイルセーフ
	// (閉じ誤判定は開誤判定より目立つため)。
	if earAvg < 0 || earAvg > 0.5 {
		return BlinkOpen
	}
	if earAvg <= earClosedThreshold {
		return BlinkClosed
	}
	if earAvg >= earOpenThreshold {
		return BlinkOpen
	}
	return BlinkHalf
}

// FaceCenterToNormalized は pixel 座標の顔中心を [-1, +1] の正規化座標に変換する。
//
// 式: (faceX / frameWidth) * 2.0 - 1.0 (Y 軸反転なし、pixel 座標そのまま)
//
// 引数:
//   - faceX, faceY: フレーム左上原点 pixel 座標 (MediaPipe output)
//   - frameWidth, frameHeight: フレーム pixel サイズ
//
// 戻り値:
//   - nx, ny: [-1, +1] 正規化座標 (中央が 0、左下が (-1, -1)、右上が (+1, +1))
//   - ok: frameWidth / frameHeight が正のとき true、ゼロ以下のとき false (呼び出し側で
//     mouse fallback すべき)
//
// mouse.Follower.Update (Phase 1.5) と同一式を採用済み。Phase 2.5 で同関数を
// 直接共有するかラッパーで揃えるかは supervisor 設計時に決定。
//
// Phase 2.4: 純粋関数・副作用なし。
func FaceCenterToNormalized(faceX, faceY float64, frameWidth, frameHeight int) (float64, float64, bool) {
	if frameWidth <= 0 || frameHeight <= 0 {
		return 0, 0, false
	}
	nx := (faceX/float64(frameWidth))*2.0 - 1.0
	ny := (faceY/float64(frameHeight))*2.0 - 1.0
	return nx, ny, true
}

// FaceDetected は「最後に顔検出成功してから現在時刻までの経過秒」が閾値未満かを返す。
//
// 戻り値:
//   - now - lastDetected < 1.0 → true  (顔検出成功とみなす)
//   - now - lastDetected >= 1.0 → false (1 秒以上未検出、fallback 推奨)
//   - 境界 (now - lastDetected == 1.0 ジャスト) → false (1.0 は閾値超過扱い)
//
// 呼び出し側は false を受け取った時点で mouse.Follower.Update にフォールバックする。
//
// 引数 now, lastDetected は Unix epoch 秒の float64。Phase 2.3 の MPClient.Latest()
// が返す DetectionJSON.Timestamp (mp_server.py 側で time.time() 出力) を lastDetected に、
// 現在時刻を now に渡す運用を想定。
//
// Phase 2.4: 純粋関数・副作用なし。
func FaceDetected(lastDetected float64, now float64) bool {
	return now-lastDetected < faceDetectionTimeoutSec
}
