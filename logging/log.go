package logging

import (
	"go.uber.org/zap"
)

var Log *zap.Logger

func init() {
	var err error
	cfg := zap.NewDevelopmentConfig()
	// stderr so nothing logged before CreateLogger can end up in a video stream written to stdout
	cfg.OutputPaths = []string{"stderr"}
	if Log, err = cfg.Build(); err != nil {
		panic(err)
	}
}

// CreateLogger replaces Log, console is "stdout", "stderr" or "" for none and level is a zap level name (debug, info, warn, error)
func CreateLogger(logFile bool, console, level string) error {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return err
	}
	cfg := zap.NewDevelopmentConfig()
	cfg.Level = lvl
	cfg.OutputPaths = make([]string, 0, 2)
	if console != "" {
		cfg.OutputPaths = append(cfg.OutputPaths, console)
	}
	if logFile {
		cfg.OutputPaths = append(cfg.OutputPaths, "./lez.log")
	}
	Log, err = cfg.Build()
	return err
}
