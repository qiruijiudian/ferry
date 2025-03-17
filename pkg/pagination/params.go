package pagination

import (
	"ferry/pkg/logger"

	"github.com/gin-gonic/gin"
)

/*
  @Author : lanyulei
*/

func RequestParams(c *gin.Context) map[string]interface{} {
	params := make(map[string]interface{}, 10)

	// if c.Request.Form == nil {
	// 	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
	// 		logger.Error(err)
	// 	}
	// }

	if c.ContentType() == "multipart/form-data" {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			logger.Error(err)
		}
	} else {
		if err := c.Request.ParseForm(); err != nil {
			logger.Error(err)
		}
	}

	// 合并 URL query 参数和表单参数
	for key, values := range c.Request.Form {
		if key == "page" || key == "per_page" || key == "sort" {
			continue
		}
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	return params
}
