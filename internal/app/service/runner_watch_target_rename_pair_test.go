package service

import (
	"iter"
	"slices"
	"testing"

	"github.com/fsnotify/fsnotify"
)

// newTestRenamePairer는 renamed에 적은 새 경로를 옛 경로의 이름을 바꾼 것으로 보는 renamePairer를 만든다.
func newTestRenamePairer(renamed map[string]string) *renamePairer {
	return newRenamePairer(func(path string) (string, bool) {
		old, ok := renamed[path]
		return old, ok
	}, newMockLogger())
}

// describePaired는 renamePairer가 돌려준 이벤트를 "CREATE b"처럼 종류와 경로로 적는다.
func describePaired(events iter.Seq[fsnotify.Event]) []string {
	var got []string
	for ev := range events {
		got = append(got, ev.Op.String()+" "+ev.Name)
	}
	return got
}

// 이름을 바꾼 항목의 Create는 옛 경로의 Rename이나 Remove가 올 때까지 뒤의 이벤트와 함께 미뤄 두었다가, 옛 경로의
// 이벤트를 먼저 돌려주고 받은 순서대로 이어서 돌려주는지 확인한다. 이름을 바꾸지 않은 항목의 Create는 곧바로 돌려준다.
func TestRenamePairerPassesOldPathBeforeRenamedCreate(t *testing.T) {
	for _, op := range []fsnotify.Op{fsnotify.Rename, fsnotify.Remove} {
		t.Run(op.String(), func(t *testing.T) {
			p := newTestRenamePairer(map[string]string{"b": "a"})
			if got, want := describePaired(p.receive(fsnotify.Event{Name: "c", Op: fsnotify.Create})), []string{"CREATE c"}; !slices.Equal(got, want) {
				t.Fatalf("create of an entry that was not renamed = %v, want %v", got, want)
			}
			for _, ev := range []fsnotify.Event{{Name: "b", Op: fsnotify.Create}, {Name: "x", Op: fsnotify.Write}} {
				if got := describePaired(p.receive(ev)); got != nil {
					t.Fatalf("%v passed on before the old path: %v", ev, got)
				}
			}
			if p.Timeout() == nil {
				t.Fatal("no timeout for the held create")
			}
			got := describePaired(p.receive(fsnotify.Event{Name: "a", Op: op}))
			if want := []string{op.String() + " a", "CREATE b", "WRITE x"}; !slices.Equal(got, want) {
				t.Fatalf("events after the old path arrived = %v, want %v", got, want)
			}
			if p.Timeout() != nil {
				t.Error("timeout is still set after the held events passed on")
			}
		})
	}
}

// 기다리는 시간이 끝나 release를 부르면 미뤄 둔 Create를 옛 경로의 이벤트 없이 돌려주고, 그 뒤에 미뤄 둔 다른 이름
// 바꾸기의 Create는 다시 옛 경로를 기다리는지 확인한다.
func TestRenamePairerReleasesHeldCreateWithoutOldPath(t *testing.T) {
	p := newTestRenamePairer(map[string]string{"b": "a", "d": "c"})
	for _, ev := range []fsnotify.Event{{Name: "b", Op: fsnotify.Create}, {Name: "x", Op: fsnotify.Write}, {Name: "d", Op: fsnotify.Create}} {
		if got := describePaired(p.receive(ev)); got != nil {
			t.Fatalf("%v passed on before the old path: %v", ev, got)
		}
	}
	if got, want := describePaired(p.release()), []string{"CREATE b", "WRITE x"}; !slices.Equal(got, want) {
		t.Fatalf("released events = %v, want %v", got, want)
	}
	if p.Timeout() == nil {
		t.Fatal("no timeout for the next held create")
	}
	if got, want := describePaired(p.receive(fsnotify.Event{Name: "c", Op: fsnotify.Rename})), []string{"RENAME c", "CREATE d"}; !slices.Equal(got, want) {
		t.Fatalf("events after the old path of the next create arrived = %v, want %v", got, want)
	}
}

// renamePairer가 다음 Create의 옛 경로를 앞에서 돌려준 이벤트를 감시 루프가 처리한 뒤에 묻는지 확인한다.
// renamedFrom이 보는 기록은 감시 루프가 이벤트를 처리하며 고친다.
func TestRenamePairerAsksRenamedFromAfterPreviousEventIsHandled(t *testing.T) {
	var log []string
	p := newRenamePairer(func(path string) (string, bool) {
		log = append(log, "ask "+path)
		if path == "b" {
			return "a", true
		}
		return "", false
	}, newMockLogger())
	for _, ev := range []fsnotify.Event{{Name: "b", Op: fsnotify.Create}, {Name: "c", Op: fsnotify.Create}, {Name: "a", Op: fsnotify.Rename}} {
		for ev := range p.receive(ev) {
			log = append(log, "handle "+ev.Name)
		}
	}
	if want := []string{"ask b", "handle a", "handle b", "ask c", "handle c"}; !slices.Equal(log, want) {
		t.Fatalf("calls = %v, want %v", log, want)
	}
}

// addSelfRename이 적은 폴더의 두 번째 Rename은 한 번만 isSelfRename으로 걸러지고, 그 경로에 항목이 새로 생겨
// forgetSelfRename으로 지운 기록은 걸러지지 않는지 확인한다.
func TestRenamePairerRecognizesSecondRenameOfMovedFolder(t *testing.T) {
	p := newTestRenamePairer(nil)
	second := fsnotify.Event{Name: "d", Op: fsnotify.Rename}
	p.addSelfRename("d")
	if !p.isSelfRename(second) {
		t.Fatal("second rename of the moved folder is not recognized")
	}
	if p.isSelfRename(second) {
		t.Fatal("record of the moved folder is still there after the second rename")
	}
	p.addSelfRename("d")
	p.forgetSelfRename("d")
	if p.isSelfRename(second) {
		t.Fatal("rename after a new entry at the path is treated as the second rename")
	}
}
