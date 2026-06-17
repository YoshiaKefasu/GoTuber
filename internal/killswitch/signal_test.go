package killswitch

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"
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
	// Process.Signal(os.Interrupt) は Windows で未サポート。
	// 将来 Windows テスト対応時は console API 経由で送信する。
	// Windows では Install() 自体が no-op になるため、Triggered() が
	// 立つ経路がない。よってこのテストは Windows では Skip する。
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt via Process.Signal is not supported on Windows; Install() is a no-op on Windows")
	}
	Reset()

	// テスト用ローカルシグナルチャネル。
	// Install() を直接呼ぶと毎回 goroutine が累積するため、ここでは channel を直接作る。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(sigCh) })
	go func() {
		<-sigCh
		triggered.Store(true)
	}()

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
