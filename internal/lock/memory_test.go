package lock

import "testing"

func TestMemoryLockerAcquireRelease(t *testing.T) {
	l := NewMemoryLocker()
	ok, owner := l.Acquire("org/repo", "apps/payments", 10)
	if !ok || owner != 10 {
		t.Fatalf("expected first acquire success: ok=%v owner=%d", ok, owner)
	}
	ok, owner = l.Acquire("org/repo", "apps/payments", 11)
	if ok || owner != 10 {
		t.Fatalf("expected conflict owner 10, got ok=%v owner=%d", ok, owner)
	}
	ok, owner = l.Acquire("org/repo", "apps/payments", 10)
	if !ok || owner != 10 {
		t.Fatalf("expected reentrant acquire by same MR")
	}
	l.ReleaseByMR("org/repo", 10)
	ok, owner = l.Acquire("org/repo", "apps/payments", 11)
	if !ok || owner != 11 {
		t.Fatalf("expected acquire after release by previous owner")
	}
}

func TestMemoryLockerList(t *testing.T) {
	l := NewMemoryLocker()
	l.Acquire("org/repo", "apps/a", 1)
	l.Acquire("org/repo", "apps/b", 2)
	locks := l.List("org/repo")
	if len(locks) != 2 || locks["apps/a"] != 1 || locks["apps/b"] != 2 {
		t.Fatalf("unexpected locks list: %+v", locks)
	}
}
