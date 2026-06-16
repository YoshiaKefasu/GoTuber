// Package mouse はマウス追従ロジックを提供する。
package mouse

// Follower はマウス位置を smoothing で追従する。
//
// 座標系: target/current 共に [-1, 1] の正規化座標。
type Follower struct {
	targetX, targetY   float64
	currentX, currentY float64
	responsiveness     float64
}

// NewFollower は新しい Follower を作成する。
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

// Update はマウス位置（ウィンドウ座標）を受け取り、target を更新して current を lerp で近づける。
// responsiveness: 0.0=動かない、1.0=即追従。値が大きいほど追従が速い。
// 動的に変更可能 (Tweaks パネルから)。
func (f *Follower) Update(mouseX, mouseY, winW, winH int, responsiveness float64) {
	if winW <= 0 || winH <= 0 {
		return
	}
	if responsiveness < 0 {
		responsiveness = 0
	}
	if responsiveness > 1 {
		responsiveness = 1
	}
	tx := float64(mouseX*2)/float64(winW) - 1
	ty := float64(mouseY*2)/float64(winH) - 1
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
	f.currentX += (f.targetX - f.currentX) * responsiveness
	f.currentY += (f.targetY - f.currentY) * responsiveness
}

// Cell は現在の表示位置 (current) を 5×5 グリッドの (row, col) にマップする。
// Y 軸反転なし: マウス上=row 0、下=row 4 (元 tomari-guruguru app.jsx:62 と完全一致)
func (f *Follower) Cell() (row, col int) {
	c := int((f.currentX + 1) / 2 * 5)
	r := int((f.currentY + 1) / 2 * 5) // Y 軸反転なし (r0=上, r4=下)
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

func (f *Follower) target() (x, y float64) {
	return f.targetX, f.targetY
}

func (f *Follower) current() (x, y float64) {
	return f.currentX, f.currentY
}
