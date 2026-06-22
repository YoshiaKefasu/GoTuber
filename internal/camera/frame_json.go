//go:build camera

package camera

// FrameJSON は CameraTracker (Phase 2.2) が port 5555 に publish する frame
// message の wire format。Phase 2.3 MPClient (SUB 5556) と Phase 2.4 Mapper で
// 再利用する想定。docs/PHASE2.md §4.3 の JSON スキーマ:
//
//	{"type":"frame","seq":N,"width":W,"height":H,"data":"<base64 JPEG>"}
//
// type フィールドは Phase 2.3 で "detection" 等の追加メッセージと区別するために必須。
type FrameJSON struct {
	Type   string `json:"type"`   // 固定値 "frame"
	Seq    uint64 `json:"seq"`    // フレーム連番 (monotonic increase)
	Width  int    `json:"width"`  // device.Bounds().Dx()
	Height int    `json:"height"` // device.Bounds().Dy()
	Data   string `json:"data"`   // base64.StdEncoding した JPEG bytes
}
