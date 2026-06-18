package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const stoplightTemplate = `
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <title>Stoplight Elements</title>
  
    <script src="https://unpkg.com/@stoplight/elements/web-components.min.js"></script>
    <link rel="stylesheet" href="https://unpkg.com/@stoplight/elements/styles.min.css">
    <style>
      html, body {
        margin: 0;
        padding: 0;
        height: 100%;
      }
      elements-api {
        display: block;
        width: 100%;
        height: 100%;
      }
    </style>
  </head>
  <body>
    <elements-api
      apiDescriptionUrl="/openapi.json"
      layout="sidebar"
      router="memory">
    </elements-api>
  </body>
</html>
`

func Stoplight(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(stoplightTemplate))
}
