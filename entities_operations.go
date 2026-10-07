package sber

import (
	"context"
	"iter"
)

func entityScopedQuery(product BankProduct, options []OperationsQuery) (OperationsQuery, error) {
	q := DefaultOperationsQuery()
	if len(options) > 1 {
		return q, NewParseError("options")
	}
	if len(options) == 1 {
		q = options[0]
	}
	resource, err := product.OperationResource()
	if err != nil {
		return q, err
	}
	if product.bankBinding() == nil {
		return q, NewParseError("product.binding")
	}
	q.Resource = resource
	return q, nil
}
func entityOperations(ctx context.Context, product BankProduct, options []OperationsQuery) ([]Operation, error) {
	q, err := entityScopedQuery(product, options)
	if err != nil {
		return nil, err
	}
	return product.bankBinding().operations.List(ctx, q)
}
func (a *BankAccount) Operations(ctx context.Context, options ...OperationsQuery) ([]Operation, error) {
	return entityOperations(ctx, a, options)
}
func (c *BankCard) Operations(ctx context.Context, options ...OperationsQuery) ([]Operation, error) {
	return entityOperations(ctx, c, options)
}

func entityIterOperations(ctx context.Context, product BankProduct, options []OperationsQuery) iter.Seq2[Operation, error] {
	return func(yield func(Operation, error) bool) {
		q, err := entityScopedQuery(product, options)
		if err != nil {
			yield(Operation{}, err)
			return
		}
		product.bankBinding().operations.Iter(ctx, q)(yield)
	}
}
func (a *BankAccount) IterOperations(ctx context.Context, options ...OperationsQuery) iter.Seq2[Operation, error] {
	return entityIterOperations(ctx, a, options)
}
func (c *BankCard) IterOperations(ctx context.Context, options ...OperationsQuery) iter.Seq2[Operation, error] {
	return entityIterOperations(ctx, c, options)
}
