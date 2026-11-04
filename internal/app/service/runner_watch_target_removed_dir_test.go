package service

import (
	"path/filepath"
	"testing"
	"time"
)

// newTestRemovedDirRecords는 *now를 현재 시각으로 보는 removedDirRecords를 만든다.
func newTestRemovedDirRecords(now *time.Time) *removedDirRecords {
	d := newRemovedDirRecords()
	d.now = func() time.Time { return *now }
	return d
}

// kqueue에서는 renamePairer가 지운 폴더의 중복 REMOVE와 후행 WRITE를 renameHoldLimit까지 미룰 수 있으므로, 기록이
// 그보다 오래 남는지 확인한다.
func TestRemovedDirTTLOutlastsRenameHold(t *testing.T) {
	if removedDirTTL <= renameHoldLimit {
		t.Fatalf("removedDirTTL = %v, want longer than renameHoldLimit %v", removedDirTTL, renameHoldLimit)
	}
}

// 중복 REMOVE가 올 수 있는 운영체제에서만, 항목 기록에 없는 경로의 REMOVE를 removedDirTTL 안에서 중복으로 보는지
// 확인한다.
func TestRemovedDirRecordsDetectRepeatedRemove(t *testing.T) {
	t.Run("repeat OS", func(t *testing.T) {
		setDirRemoveRepeat(t, true)
		now := time.Now()
		d := newTestRemovedDirRecords(&now)
		d.add("pkg")
		if !d.isRepeatedRemove("pkg", false) {
			t.Error("remove of the removed folder is not a repeat")
		}
		if d.isRepeatedRemove("pkg", true) {
			t.Error("remove of a recorded path is a repeat")
		}
		if d.isRepeatedRemove("other", false) {
			t.Error("remove of a path that was not removed is a repeat")
		}
		now = now.Add(removedDirTTL)
		if !d.isRepeatedRemove("pkg", false) {
			t.Error("remove at removedDirTTL is not a repeat")
		}
		now = now.Add(time.Millisecond)
		if d.isRepeatedRemove("pkg", false) {
			t.Error("remove after removedDirTTL is a repeat")
		}
	})
	t.Run("other OS", func(t *testing.T) {
		setDirRemoveRepeat(t, false)
		d := newRemovedDirRecords()
		d.add("pkg")
		if d.isRepeatedRemove("pkg", false) {
			t.Error("remove is a repeat where removes are not repeated")
		}
	})
}

// 지운 폴더의 WRITE를 디스크에서 확인해, 경로가 없으면 버리고 새 항목이 있으면 재생성으로 보며 기록을 지우는지
// 확인한다. 지운 폴더가 아니거나 removedDirTTL이 지난 경로의 WRITE는 어느 쪽도 아니다.
func TestRemovedDirRecordsClassifyWrite(t *testing.T) {
	setDirRemoveRepeat(t, true)
	root := t.TempDir()
	gone, recreated := filepath.Join(root, "gone"), filepath.Join(root, "recreated")
	now := time.Now()
	d := newTestRemovedDirRecords(&now)
	d.add(gone)
	d.add(recreated)
	writeFile(t, recreated)
	check := func(path string, wantStale, wantRecreated bool) {
		t.Helper()
		if stale, recreated := d.classifyWrite(path); stale != wantStale || recreated != wantRecreated {
			t.Errorf("classifyWrite(%s) = stale %v, recreated %v, want %v, %v", filepath.Base(path), stale, recreated, wantStale, wantRecreated)
		}
	}
	check(gone, true, false)
	check(recreated, false, true)
	check(recreated, false, false)
	check(filepath.Join(root, "other"), false, false)
	now = now.Add(removedDirTTL + time.Millisecond)
	check(gone, false, false)
}

// path에 항목이 새로 생기면 path와 그 아래 경로의 기록만 지우는지 확인한다.
func TestRemovedDirRecordsForgetUnderCreatedPath(t *testing.T) {
	setDirRemoveRepeat(t, true)
	d := newRemovedDirRecords()
	for _, path := range []string{"pkg", filepath.Join("pkg", "sub"), "pkg2"} {
		d.add(path)
	}
	d.forgetUnder("pkg")
	for path, want := range map[string]bool{"pkg": false, filepath.Join("pkg", "sub"): false, "pkg2": true} {
		if got := d.isRepeatedRemove(path, false); got != want {
			t.Errorf("record of %s after the create = %v, want %v", path, got, want)
		}
	}
}

// removedDirTTL이 지난 기록은 다음 폴더를 적을 때 지워, 이름이 매번 다른 폴더를 지워도 기록이 쌓이지 않는지
// 확인한다.
func TestRemovedDirRecordsDropExpired(t *testing.T) {
	setDirRemoveRepeat(t, true)
	now := time.Now()
	d := newTestRemovedDirRecords(&now)
	d.add("old")
	now = now.Add(removedDirTTL + time.Millisecond)
	d.add("new")
	if _, ok := d.at["old"]; ok || len(d.at) != 1 {
		t.Errorf("records = %v, want only new", d.at)
	}
}
