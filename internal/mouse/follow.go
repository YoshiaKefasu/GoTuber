// Package mouse はマウス追従ロジックを提供する。
package mouse

import "math"

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
// 元 tomari-guruguru app.jsx:61-62 と同じ式: `Math.round((current+1)/2 * (N-1))`、
// Go 側では `int(math.Round(...))` で `Math.round` の最近接丸めを再現 (y=上=row 0, y=下=row 4)。
func (f *Follower) Cell() (row, col int) {
	c := int(math.Round((f.currentX + 1) / 2 * 4)) // 4 = COLS - 1
	r := int(math.Round((f.currentY + 1) / 2 * 4)) // 4 = ROWS - 1
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

// Current は現在の追従位置を正規化座標 [-1, 1] で返す。
// x: 左=-1.0 / 中央=0.0 / 右=+1.0 (yaw 方向)
// y: 上=-1.0 / 中央=0.0 / 下=+1.0 (pitch 方向)
//
// Phase 4.2: depth-weighted elastic morph の入力源として使用。
func (f *Follower) Current() (x, y float64) {
	return f.currentX, f.currentY
}
