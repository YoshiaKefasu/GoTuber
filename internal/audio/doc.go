// Package audio はマイクからの音声キャプチャ + RMS エンベロープ + 口パクマッピングを提供する。
//
// パイプライン:
//
//	mic (PCM int16) ─[Capture]─▶ RMS (0..1) ─[EnvelopeFollower]─▶ 平滑化 RMS
//	                          ─[MouthTracker]─▶ MouthState (0=closed, 1=half, 2=open)
//
// スレッド:
//   - Capture の RMS 更新: malgo オーディオスレッド
//   - GetRMS / EnvelopeFollower.Update / MouthTracker.Update: game スレッド
//   - RMS は atomic uint64 でやり取り（mutex なし）
//
// フェイルセーフ: NewMover がエラー（デバイスなし等）を返したら main は mover=nil で続行可。
// game.Update は mover==nil をチェックして口パクをスキップする。
package audio
