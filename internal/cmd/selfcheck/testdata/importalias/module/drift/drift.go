package drift

import (
	"example.com/module/logger"
	slogger "example.com/module/sink/logger"
)

func Use() string { return logger.New() + slogger.New() }
