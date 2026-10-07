package bank

import (
	"context"
)

func (c *BankCard) Rename(ctx context.Context, name string) error {
	binding := c.bankBinding()
	if binding == nil {
		return NewParseError("card.binding")
	}
	return binding.cards.Rename(ctx, c.ID(), name)
}

// Transfers exposes the same guarded workflow owner used by TransferTo, so a
// standalone constructed portfolio can explicitly confirm its prepared result.
func (p *BankPortfolio) Transfers() *TransfersAPI {
	if p == nil || p.data == nil {
		return nil
	}
	return (*p.data).binding.transfers
}
func entityTransferTo(ctx context.Context, source, destination BankProduct, amount Decimal, options []TransferOptions) (PreparedTransfer, error) {
	binding := source.bankBinding()
	if binding == nil {
		return PreparedTransfer{}, NewParseError("source.binding")
	}
	if destination == nil || destination.bankBinding() == nil {
		return PreparedTransfer{}, NewParseError("destination")
	}
	other := destination.bankBinding()
	if binding != other && !entitySameRequester(binding.requester, other.requester) {
		return PreparedTransfer{}, NewParseError("destination.client")
	}
	if !binding.options.AllowMutations {
		return PreparedTransfer{}, ErrResourceMutationsDisabled
	}
	sourceID, err := source.TransferResource()
	if err != nil {
		return PreparedTransfer{}, err
	}
	destinationID, err := destination.TransferResource()
	if err != nil {
		return PreparedTransfer{}, err
	}
	if sourceID == destinationID {
		return PreparedTransfer{}, NewParseError("destination_id")
	}
	if _, err = resourceTransferAmount(amount); err != nil {
		return PreparedTransfer{}, err
	}
	draft, err := binding.transfers.Start(ctx)
	if err != nil {
		return PreparedTransfer{}, err
	}
	return binding.transfers.Prepare(ctx, draft, sourceID, destinationID, amount, options...)
}
func (a *BankAccount) TransferTo(ctx context.Context, destination BankProduct, amount Decimal, options ...TransferOptions) (PreparedTransfer, error) {
	return entityTransferTo(ctx, a, destination, amount, options)
}
func (c *BankCard) TransferTo(ctx context.Context, destination BankProduct, amount Decimal, options ...TransferOptions) (PreparedTransfer, error) {
	return entityTransferTo(ctx, c, destination, amount, options)
}
