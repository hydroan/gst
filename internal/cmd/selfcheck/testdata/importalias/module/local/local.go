package local

import sinklogger "example.com/module/sink/logger"

func Use() string {
	logger := "local"
	return logger + sinklogger.New()
}
