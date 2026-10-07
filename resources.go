// Typed resources ported from the MIT-licensed pinned sber-unofficial SDK.
package sber

import "context"

const ProductsPath = "/main-screen/rest/v2/m1/web/section/meta"

// ProductsAPI reads one product snapshot, without implicit warmup or filtering.
type ProductsAPI struct{ requester BusinessRequester }

func NewProductsAPI(requester BusinessRequester) *ProductsAPI {
	return &ProductsAPI{requester: requester}
}
func (a *ProductsAPI) Get(ctx context.Context, forceUpdate bool) (Products, error) {
	payload, err := a.requester.PostRead(ctx, ProductsPath, map[string]any{"withData": true, "forceUpdate": forceUpdate})
	if err != nil {
		return Products{}, err
	}
	return ParseProducts(payload)
}
