package field

import sinklogger "example.com/module/sink/logger"

type holder struct{ logger string }

func Use() string { return holder{logger: sinklogger.New()}.logger }
