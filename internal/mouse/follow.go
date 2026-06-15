// Package mouse はマウス追従ロジックを提供する。
// ウィンドウ座標を [-1, 1] の正規化座標に変換し、
// responsiveness に応じた lerp で滑らかに追従する。
package mouse

// Follower はマウス追従の現在状態とロジックを保持する。
//
// 座標系: target/current 共に [-1, 1] の正規化座標。
//   - (-1, -1) = ウィンドウ左上
//   - ( 0,  0) = ウィンドウ中央
//   - ( 1,  1) = ウィンドウ右下
//
// Y 軸は「マウスの上方向 = current.y = -1」とする（ウィンドウ座標の上=小さい y）。
// Cell() では Y 軸を反転し、original tomari-guruguru と同様に
// 「マウス下方向 = row 0」「マウス上方向 = row 4」となる。
type Follower struct {
	targetX, targetY   float64 // 目標位置（クランプ後、[-1, 1]）
	currentX, currentY float64 // 現在表示位置（lerp で target に追従）
	responsiveness     float64 // 1 フレームあたりの追従率 (0=動かない, 1=即追従)
}

// NewFollower は新しい Follower を作成する。
// responsiveness: 0.0 (停止) 〜 1.0 (即追従)、推奨 0.2〜0.5。
// 範囲外の値はクランプされる。
func NewFollower(responsiveness float64) *Follower {
	if responsiveness < 0 {
		responsiveness = 0
	}
	if responsiveness > 1 {
		responsiveness = 1
	}
	return &Follower{
		responsiveness: responsiveness,
	}
}

// Update はマウス位置（ウィンドウ座標）を受け取り、target を更新して
// current を lerp で target に近づける。
// winW / winH が 0 以下の場合は何もしない。
func (f *Follower) Update(mouseX, mouseY, winW, winH int) {
	if winW <= 0 || winH <= 0 {
		return
	}
	// [-1, 1] にマップ
	tx := float64(mouseX*2)/float64(winW) - 1
	ty := float64(mouseY*2)/float64(winH) - 1
	// クランプ
	if tx < -1 {
		tx = -1
	}
	if tx > 1 {
		tx = 1
	}
	if ty < -1 {
		ty = -1
	}
	if ty > 1 {
		ty = 1
	}
	f.targetX, f.targetY = tx, ty
	// lerp
	f.currentX += (f.targetX - f.currentX) * f.responsiveness
	f.currentY += (f.targetY - f.currentY) * f.responsiveness
}

// Cell は現在の表示位置 (current) を 5×5 グリッドの (row, col) にマップする。
//   - row 0 = マウス下方向、row 4 = マウス上方向（Y 軸反転）
//   - col 0 = マウス左方向、col 4 = マウス右方向
//   - 戻り値: 0〜4
func (f *Follower) Cell() (row, col int) {
	c := int((f.currentX + 1) / 2 * 5)
	r := 4 - int((f.currentY + 1) / 2 * 5) // Y 軸反転
	if c < 0 {
		c = 0
	}
	if c > 4 {
		c = 4
	}
	if r < 0 {
		r = 0
	}
	if r > 4 {
		r = 4
	}
	return r, c
}

// Target は現在の target を返す（テスト・デバッグ用）。
func (f *Follower) Target() (x, y float64) {
	return f.targetX, f.targetY
}

// Current は現在の current を返す（テスト・デバッグ用）。
func (f *Follower) Current() (x, y float64) {
	return f.currentX, f.currentY
}
