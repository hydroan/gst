package ident

import sinklogger "example.com/module/sink/logger"

func Use() string { return logger + sinklogger.New() }
