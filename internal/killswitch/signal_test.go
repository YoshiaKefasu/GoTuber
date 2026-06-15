package killswitch

import (
	"os"
	"testing"
	"time"
)

func TestTriggeredInitiallyFalse(t *testing.T) {
	Reset()
	if Triggered() {
		t.Error("expected Triggered() to be false initially")
	}
}

func TestSignalTriggersKillSwitch(t *testing.T) {
	Reset()
	Install()

	// 自分自身のプロセスに SIGINT 送信
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess failed: %v", err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Fatalf("Signal(SIGINT) failed: %v", err)
	}

	// シグナル伝播を待つ
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if Triggered() {
			return // OK
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Error("expected Triggered() to become true within 2s after SIGINT")
}

func TestEscTriggersKillSwitch(t *testing.T) {
	// Esc 検出は Ebitengine inpututil に依存するため統合テスト扱い。
	// ここでは状態遷移のみを検証（escPressed が立ったら Triggered() が true）。
	Reset()
	escPressed.Store(true)
	if !Triggered() {
		t.Error("expected Triggered() to be true when escPressed is set")
	}
}
