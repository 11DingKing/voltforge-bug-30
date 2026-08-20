package audit

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"net/http"
	"os"
	"runtime/debug"
	"time"
)

type Logger struct {
	logger zerolog.Logger
}

func NewLogger(level string) *Logger {
	zerolog.TimeFieldFormat = time.RFC3339
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	return &Logger{
		logger: zerolog.New(osStdout()).Level(lvl).With().Timestamp().Logger(),
	}
}

func (l *Logger) Info() *zerolog.Event  { return l.logger.Info() }
func (l *Logger) Warn() *zerolog.Event  { return l.logger.Warn() }
func (l *Logger) Error() *zerolog.Event { return l.logger.Error() }
func (l *Logger) Debug() *zerolog.Event { return l.logger.Debug() }

type contextKey string

const RequestIDKey contextKey = "request_id"

func (l *Logger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		ww := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			if rec := recover(); rec != nil {
				l.logger.Error().
					Str("request_id", requestID).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Interface("panic", rec).
					Bytes("stack", debug.Stack()).
					Msg("panic recovered")
				http.Error(ww, "internal server error", http.StatusInternalServerError)
			}
			l.logger.Info().
				Str("request_id", requestID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.status).
				Dur("latency", time.Since(start)).
				Msg("request completed")
		}()
		next.ServeHTTP(ww, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func osStdout() *os.File { return os.Stdout }
