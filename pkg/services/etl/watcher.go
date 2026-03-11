package etl

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FileHandler is called when a new file is detected in a watched directory.
type FileHandler func(ctx context.Context, projectID uuid.UUID, appID, filePath string)

// Watcher monitors directories for new files and dispatches to handlers.
type Watcher struct {
	mu       sync.Mutex
	watchers map[string]*fsnotify.Watcher // keyed by projectID:appID
	handlers map[string]FileHandler       // keyed by file extension
	logger   *zap.Logger
	cancel   context.CancelFunc
}

// NewWatcher creates a new directory watcher.
func NewWatcher(logger *zap.Logger) *Watcher {
	return &Watcher{
		watchers: make(map[string]*fsnotify.Watcher),
		handlers: make(map[string]FileHandler),
		logger:   logger,
	}
}

// RegisterHandler registers a handler for specific file extensions.
func (w *Watcher) RegisterHandler(extensions []string, handler FileHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ext := range extensions {
		w.handlers[strings.ToLower(ext)] = handler
	}
}

// Watch starts watching a directory for a specific project/app.
func (w *Watcher) Watch(ctx context.Context, projectID uuid.UUID, appID, directory string) error {
	key := projectID.String() + ":" + appID

	w.mu.Lock()
	// Stop existing watcher for this project/app
	if existing, ok := w.watchers[key]; ok {
		existing.Close()
		delete(w.watchers, key)
	}
	w.mu.Unlock()

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := fsWatcher.Add(directory); err != nil {
		fsWatcher.Close()
		return err
	}

	w.mu.Lock()
	w.watchers[key] = fsWatcher
	w.mu.Unlock()

	go w.watchLoop(ctx, fsWatcher, projectID, appID, key)

	w.logger.Info("started watching directory",
		zap.String("directory", directory),
		zap.String("project_id", projectID.String()),
		zap.String("app_id", appID),
	)

	return nil
}

func (w *Watcher) watchLoop(ctx context.Context, fsWatcher *fsnotify.Watcher, projectID uuid.UUID, appID, key string) {
	defer func() {
		w.mu.Lock()
		delete(w.watchers, key)
		w.mu.Unlock()
		fsWatcher.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}

			ext := strings.ToLower(filepath.Ext(event.Name))
			w.mu.Lock()
			handler, ok := w.handlers[ext]
			w.mu.Unlock()

			if ok {
				w.logger.Info("file detected",
					zap.String("file", event.Name),
					zap.String("ext", ext),
				)
				go handler(ctx, projectID, appID, event.Name)
			}

		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("watcher error",
				zap.String("key", key),
				zap.Error(err),
			)
		}
	}
}

// StopWatch stops watching for a specific project/app.
func (w *Watcher) StopWatch(projectID uuid.UUID, appID string) {
	key := projectID.String() + ":" + appID
	w.mu.Lock()
	defer w.mu.Unlock()

	if watcher, ok := w.watchers[key]; ok {
		watcher.Close()
		delete(w.watchers, key)
	}
}

// Close stops all watchers.
func (w *Watcher) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for key, watcher := range w.watchers {
		watcher.Close()
		delete(w.watchers, key)
	}
}
