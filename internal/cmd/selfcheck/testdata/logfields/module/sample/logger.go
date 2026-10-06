package sample

type logger struct{}

func (logger) Infow(msg string, keysValues ...any) {}

func (logger) With(keysValues ...any) logger { return logger{} }

var log logger
