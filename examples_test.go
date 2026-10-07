package sber_test

import (
	"fmt"

	sber "github.com/vasyza/sber-go"
)

func ExampleParseDecimal() {
	amount, err := sber.ParseDecimal("9007199254740993,10")
	if err != nil {
		panic(err)
	}
	fmt.Println(amount.String())
	// Output: 9007199254740993.10
}

func ExampleParseProducts() {
	payload := map[string]any{"success": true, "body": map[string]any{"sections": map[string]any{"technicalSection": map[string]any{"sectionProductData": map[string]any{"ctaccounts": map[string]any{"data": []any{map[string]any{"id": "synthetic-account", "balance": map[string]any{"amount": "10.50", "currencyCode": "RUB"}}}}}}}}}
	products, err := sber.ParseProducts(payload)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(products.Accounts), products.Accounts[0].Balance.Amount.String())
	// Output: 1 10.50
}
