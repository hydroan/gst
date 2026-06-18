package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	redocSpecURLPlaceholder = `{RedocSpecUrl}`
	redocTemplate           = `
<!DOCTYPE html>
<html>
  <head>
    <title>API Reference</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>
      body { margin: 0; padding: 0; }
    </style>
  </head>
  <body>
    <redoc spec-url="{RedocSpecUrl}" show-object-schema-examples="true"></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
  </body>
</html>
`
)

func Redoc(c *gin.Context) {
	content := replaceByMap(redocTemplate, map[string]string{
		redocSpecURLPlaceholder: "/openapi.json",
	})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func replaceByMap(origin string, replaces map[string]string) string {
	for k, v := range replaces {
		origin = strings.ReplaceAll(origin, k, v)
	}
	return origin
}
