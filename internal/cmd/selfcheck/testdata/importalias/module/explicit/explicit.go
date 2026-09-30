package explicit

import namedimpl "example.com/module/named/impl"

func Use() string { return namedimpl.Name() }
