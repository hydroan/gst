package needed

import (
	"example.com/module/logger"
	sinklogger "example.com/module/sink/logger"
)

func Use() string { return logger.New() + sinklogger.New() }
