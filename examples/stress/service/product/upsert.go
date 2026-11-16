package product

import (
	"net/http"

	"stress/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Upsert serves POST /api/products/upsert: the product is inserted, or
// overwritten where its code is taken. The row is named by its code,
// productID, so the merge meets the primary key, which every dialect merges
// on.
type Upsert struct {
	service.Base[*model.Product, *model.ProductUpsertReq, *model.Product]
}

func (u *Upsert) Create(ctx *gst.ServiceContext, req *model.ProductUpsertReq) (*model.Product, error) {
	product := &model.Product{Code: req.Code, Title: req.Title, Category: req.Category, Status: req.Status, Score: req.Score, Body: req.Body}
	product.ID = productID(req.Code)
	if err := database.Database[*model.Product](ctx).Upsert(product); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "upsert product failed", err)
	}
	return product, nil
}

// productID is the id of the product carrying code: "sku-" and the code. The
// rows the scenarios seed carry the same id, so an upsert meets them.
func productID(code string) string { return "sku-" + code }
