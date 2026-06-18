package state

import (
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()

	f, err := os.CreateTemp("", "davit-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()

	db, err := Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("open db: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.Remove(f.Name())
	})

	return db
}

func seedTestApp(t *testing.T, db *DB, name string) {
	t.Helper()
	err := db.CreateApp(App{
		Name:          name,
		RepoURL:       "https://github.com/test/" + name,
		Branch:        "main",
		Domain:        name + ".example.com",
		ContainerPort: 3000,
		InternalPort:  40000,
		Status:        "running",
	})
	if err != nil {
		t.Fatalf("seed app %s: %v", name, err)
	}
}

func TestGetActiveWatchers_Empty(t *testing.T) {
	db := newTestDB(t)
	watchers, err := db.GetActiveWatchers()
	if err != nil {
		t.Fatalf("GetActiveWatchers: %v", err)
	}
	if len(watchers) != 0 {
		t.Fatalf("expected 0 watchers, got %d", len(watchers))
	}
}

func TestGetActiveWatchers_WithData(t *testing.T) {
	db := newTestDB(t)

	seedTestApp(t, db, "test-app-1")
	seedTestApp(t, db, "test-app-2")

	// Enable watch on test-app-1 (polling)
	err := db.UpdateAppWatch("test-app-1", true, 30, false)
	if err != nil {
		t.Fatalf("UpdateAppWatch: %v", err)
	}

	// Enable watch on test-app-2 (webhook)
	err = db.UpdateAppWatch("test-app-2", true, 0, true)
	if err != nil {
		t.Fatalf("UpdateAppWatch (2): %v", err)
	}

	watchers, err := db.GetActiveWatchers()
	if err != nil {
		t.Fatalf("GetActiveWatchers: %v", err)
	}
	if len(watchers) != 2 {
		t.Fatalf("expected 2 watchers, got %d", len(watchers))
	}

	// Verify first watcher
	if watchers[0].Method != "polling" && watchers[1].Method != "polling" {
		t.Fatal("expected a polling watcher")
	}
	if watchers[0].Method != "webhook" && watchers[1].Method != "webhook" {
		t.Fatal("expected a webhook watcher")
	}

	// Verify polling watcher has interval set
	for _, w := range watchers {
		if w.Method == "polling" && w.PollIntervalSeconds != 30 {
			t.Fatalf("expected poll interval 30, got %d", w.PollIntervalSeconds)
		}
	}
}

func TestGetWatcher_NotFound(t *testing.T) {
	db := newTestDB(t)
	w, err := db.GetWatcher("nonexistent")
	if err != nil {
		t.Fatalf("GetWatcher: %v", err)
	}
	if w != nil {
		t.Fatal("expected nil watcher for nonexistent app")
	}
}

func TestGetWatcher_Found(t *testing.T) {
	db := newTestDB(t)
	seedTestApp(t, db, "test-app")

	err := db.UpdateAppWatch("test-app", true, 60, false)
	if err != nil {
		t.Fatalf("UpdateAppWatch: %v", err)
	}

	w, err := db.GetWatcher("test-app")
	if err != nil {
		t.Fatalf("GetWatcher: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil watcher")
	}
	if w.AppName != "test-app" {
		t.Fatalf("expected app name 'test-app', got %q", w.AppName)
	}
	if w.Method != "polling" {
		t.Fatalf("expected method 'polling', got %q", w.Method)
	}
	if w.PollIntervalSeconds != 60 {
		t.Fatalf("expected poll interval 60, got %d", w.PollIntervalSeconds)
	}
	if w.Status != "active" {
		t.Fatalf("expected status 'active', got %q", w.Status)
	}
}

func TestUpdateWatcherCommit(t *testing.T) {
	db := newTestDB(t)
	seedTestApp(t, db, "test-app")

	err := db.UpdateAppWatch("test-app", true, 30, false)
	if err != nil {
		t.Fatalf("UpdateAppWatch: %v", err)
	}

	// Update commit
	commitSHA := "abc123def456"
	err = db.UpdateWatcherCommit("test-app", commitSHA)
	if err != nil {
		t.Fatalf("UpdateWatcherCommit: %v", err)
	}

	// Verify
	w, err := db.GetWatcher("test-app")
	if err != nil {
		t.Fatalf("GetWatcher: %v", err)
	}
	if w.LastCommitSHA != commitSHA {
		t.Fatalf("expected commit %q, got %q", commitSHA, w.LastCommitSHA)
	}
	if w.LastCheckedAt.IsZero() {
		t.Fatal("expected LastCheckedAt to be set")
	}
}

func TestUpdateWatcherCommit_UpdatesTimestamp(t *testing.T) {
	db := newTestDB(t)
	seedTestApp(t, db, "test-app")
	err := db.UpdateAppWatch("test-app", true, 30, false)
	if err != nil {
		t.Fatalf("UpdateAppWatch: %v", err)
	}

	// Update with first commit
	err = db.UpdateWatcherCommit("test-app", "sha1")
	if err != nil {
		t.Fatalf("first UpdateWatcherCommit: %v", err)
	}

	w1, _ := db.GetWatcher("test-app")

	// Update with different commit
	err = db.UpdateWatcherCommit("test-app", "sha2")
	if err != nil {
		t.Fatalf("second UpdateWatcherCommit: %v", err)
	}

	w2, _ := db.GetWatcher("test-app")

	// Both timestamps should be non-zero
	if w1.LastCheckedAt.IsZero() {
		t.Fatal("expected first LastCheckedAt to be set")
	}
	if w2.LastCheckedAt.IsZero() {
		t.Fatal("expected second LastCheckedAt to be set")
	}
	// The commit SHA should have been updated
	if w1.LastCommitSHA != "sha1" {
		t.Fatalf("expected first commit 'sha1', got %q", w1.LastCommitSHA)
	}
	if w2.LastCommitSHA != "sha2" {
		t.Fatalf("expected second commit 'sha2', got %q", w2.LastCommitSHA)
	}
}

func TestGetActiveWatchers_ExcludesInactive(t *testing.T) {
	db := newTestDB(t)
	seedTestApp(t, db, "app-1")
	seedTestApp(t, db, "app-2")

	// Enable watch on both
	_ = db.UpdateAppWatch("app-1", true, 30, false)
	_ = db.UpdateAppWatch("app-2", true, 30, false)

	// Disable app-1
	_ = db.UpdateAppWatch("app-1", false, 0, false)

	watchers, err := db.GetActiveWatchers()
	if err != nil {
		t.Fatalf("GetActiveWatchers: %v", err)
	}
	if len(watchers) != 1 {
		t.Fatalf("expected 1 active watcher, got %d", len(watchers))
	}
	if watchers[0].AppName != "app-2" {
		t.Fatalf("expected active watcher for 'app-2', got %q", watchers[0].AppName)
	}
}

func TestGetActiveWatchers_WebhookToken(t *testing.T) {
	db := newTestDB(t)
	seedTestApp(t, db, "test-app")

	err := db.UpdateAppWatch("test-app", true, 0, true)
	if err != nil {
		t.Fatalf("UpdateAppWatch: %v", err)
	}

	w, err := db.GetWatcher("test-app")
	if err != nil {
		t.Fatalf("GetWatcher: %v", err)
	}
	if w.WebhookToken == "" {
		t.Fatal("expected non-empty webhook token for webhook mode")
	}
	if w.Method != "webhook" {
		t.Fatalf("expected method 'webhook', got %q", w.Method)
	}
}