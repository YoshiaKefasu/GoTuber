package audio

import "testing"

// TestDevice_Struct は audio.Device struct がフィールドを保持できることを確認。
func TestDevice_Struct(t *testing.T) {
	d := Device{
		ID:   "{0.0.0.00000000}.{abc-def-1234}",
		Name: "USB Microphone",
	}
	if d.ID == "" {
		t.Error("expected non-empty ID")
	}
	if d.Name == "" {
		t.Error("expected non-empty Name")
	}
}

// TestDevice_ZeroValue はゼロ値でも問題が出ないことを確認。
// Phase 1.13a.7: 表示名重複対策で ID ベースで管理するため、
// 空 ID は "OS デフォルト" として扱う設計。
func TestDevice_ZeroValue(t *testing.T) {
	d := Device{}
	if d.ID != "" {
		t.Errorf("expected zero-value ID=\"\", got %q", d.ID)
	}
	if d.Name != "" {
		t.Errorf("expected zero-value Name=\"\", got %q", d.Name)
	}
}

// TestListDevices_DoesNotPanic は実機環境でも panic しないことを確認。
// 実機依存 (デバイス 0 個や malgo バックエンド未対応) の場合は
// error を返すが、panic しないことだけが要件。
func TestListDevices_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ListDevices panicked: %v", r)
		}
	}()
	_, _ = ListDevices() // エラーは許容 (環境依存)
}
