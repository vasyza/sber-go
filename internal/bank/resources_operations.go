package bank

import (
	"context"
	"iter"
	"regexp"
	"time"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

const OperationsPath = "/uoh-bh/v1/operations/list"

// ResourceOptions is copied by constructors. Writes stay disabled unless the
// caller opts in; the requester must independently enforce its own policy.
// Now is used only to freeze the source's default 120-day Moscow window.
type ResourceOptions struct {
	AllowMutations bool
	Now            func() time.Time
}

func resourceOptions(options []ResourceOptions) ResourceOptions {
	// More than one constructor option fails closed on writes.
	o := ResourceOptions{}
	if len(options) == 1 {
		o = options[0]
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

var resourceOperationGrammar = regexp.MustCompile(`^(?:card|ct-account|account):[A-Za-z0-9_-]{1,128}$`)

// Dates accept date-only, naive Moscow ISO, or aware ISO timestamps. A
// date-only To is inclusive through 23:59:59, not the next midnight.
// Explicit options must carry a valid Limit; zero is not silently repaired.
type OperationsPageOptions struct {
	Resource      string
	Offset, Limit int
	From, To      string
}

func DefaultOperationsPageOptions() OperationsPageOptions { return OperationsPageOptions{Limit: 30} }

type OperationsAPI struct {
	requester BusinessRequester
	options   ResourceOptions
}

func NewOperationsAPI(requester BusinessRequester, options ...ResourceOptions) *OperationsAPI {
	return &OperationsAPI{requester, resourceOptions(options)}
}
func (a *OperationsAPI) pageOptions(options []OperationsPageOptions) (OperationsPageOptions, error) {
	q := DefaultOperationsPageOptions()
	if len(options) > 1 {
		return q, NewParseError("options")
	}
	if len(options) == 1 {
		q = options[0]
	}
	if q.Resource != "" && !resourceOperationGrammar.MatchString(q.Resource) {
		return q, NewParseError("resource")
	}
	if q.Offset < 0 || q.Limit < 1 || q.Limit > 100 || q.Offset > int(^uint(0)>>1)-q.Limit {
		return q, NewParseError("pagination")
	}
	if q.From == "" && q.To == "" {
		// The source freezes datetime.now(Moscow), strips tzinfo, then
		// subtracts 120 CIVIL days. Do not attach a rounded historical
		// offset or let native AddDate choose an imaginary/fold instant.
		now := a.options.Now().In(domainMoscow)
		wall := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond()/1000*1000, time.UTC)
		q.From = wall.AddDate(0, 0, -120).Format("2006-01-02T15:04:05.999999")
	}
	return q, nil
}
func (a *OperationsAPI) Page(ctx context.Context, options ...OperationsPageOptions) (OperationsPage, error) {
	if err := ctx.Err(); err != nil {
		return OperationsPage{}, err
	}
	q, err := a.pageOptions(options)
	if err != nil {
		return OperationsPage{}, err
	}
	f, err := NewSourceTimeFilter(q.From, q.To)
	if err != nil {
		return OperationsPage{}, err
	}
	from, to, err := f.SourceRequestBounds()
	if err != nil {
		return OperationsPage{}, err
	}
	body := map[string]any{"paginationOffset": q.Offset, "paginationSize": q.Limit + 1, "showHidden": false, "showNotTransactionBonuses": true, "showOpenBanking": true}
	if from != "" {
		body["from"] = from
	}
	if to != "" {
		body["to"] = to
	}
	if q.Resource != "" {
		body["usedResource"] = []string{q.Resource}
	}
	payload, err := a.requester.PostRead(ctx, OperationsPath, body)
	if err != nil {
		return OperationsPage{}, err
	}
	scopes := []string{}
	if q.Resource != "" {
		scopes = append(scopes, q.Resource)
	}
	operations, err := ParseOperations(payload, scopes...)
	if err != nil {
		return OperationsPage{}, err
	}
	for _, op := range operations {
		if op.ID == "" {
			return OperationsPage{}, &sdkErrs.APIError{Message: "operation missing stable uohId"}
		}
	}
	page := OperationsPage{Operations: operations}
	if len(operations) > q.Limit {
		next := q.Offset + q.Limit
		page.NextOffset = &next
		page.Operations = operations[:q.Limit]
	}
	return page, nil
}

// OperationsQuery starts at offset zero, just like the reference iterator.
type OperationsQuery struct {
	Resource string
	Limit    int
	From, To string
	MaxPages int
}

func DefaultOperationsQuery() OperationsQuery { return OperationsQuery{Limit: 30, MaxPages: 100} }

// List sorts newest first, stably for equal/invalid dates. Failure returns nil,
// not a successful partial slice; use Iter to deliberately consume partials.
func (a *OperationsAPI) List(ctx context.Context, options ...OperationsQuery) ([]Operation, error) {
	out := []Operation{}
	for op, err := range a.Iter(ctx, options...) {
		if err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return SortSourceOperations(out), nil
}

// SortSourceOperations returns a shallow value copy in canonical Python3.12.3
// sorted(..., reverse=True) order. SourceOperationCompare is not transitive on
// mixed timezone identities, so the comparison SCHEDULE is part of the source
// contract. This native port retains CPython's reverse/run/binary insertion,
// powersort stack, directional merges and adaptive gallop schedule. It does
// not invoke Python, invent instants, normalize operation dates or mutate input.
// Algorithm adapted from CPython v3.12.3 Objects/listobject.c, PSF License 2;
// copyright (c) 2001-2023 Python Software Foundation, all rights reserved.
// The retained license and modification summary are in DATETIME-CYCLE4.md.
func SortSourceOperations(operations []Operation) []Operation {
	out := make([]Operation, len(operations))
	if len(operations) < 2 {
		copy(out, operations)
		return out
	}
	s := domainSourceSorter{items: make([]domainSourceSortItem, len(operations)), minGallop: 7}
	for i, op := range operations {
		s.items[i] = domainSourceSortItem{op: op, key: domainOperationSourceKey(op.Date)}
	}
	domainSourceReverse(s.items)
	n, remainder := len(s.items), 0
	for n >= 64 {
		remainder |= n & 1
		n >>= 1
	}
	minrun := n + remainder
	for start := 0; start < len(s.items); {
		run := s.countRun(start)
		if run < minrun {
			force := min(minrun, len(s.items)-start)
			s.binarySort(start, start+force, start+run)
			run = force
		}
		if len(s.pending) != 0 {
			top := s.pending[len(s.pending)-1]
			power := domainSourcePower(top.start, top.length, run, len(s.items))
			for len(s.pending) > 1 && s.pending[len(s.pending)-2].power > power {
				s.mergeAt(len(s.pending) - 2)
			}
			s.pending[len(s.pending)-1].power = power
		}
		s.pending = append(s.pending, domainSourceRun{start: start, length: run})
		start += run
	}
	for len(s.pending) > 1 {
		i := len(s.pending) - 2
		if i > 0 && s.pending[i-1].length < s.pending[i+1].length {
			i--
		}
		s.mergeAt(i)
	}
	domainSourceReverse(s.items)
	for i, item := range s.items {
		out[i] = item.op
	}
	return out
}

type domainSourceSortItem struct {
	op  Operation
	key SourceDateTime
}
type domainSourceRun struct{ start, length, power int }
type domainSourceSorter struct {
	items     []domainSourceSortItem
	pending   []domainSourceRun
	minGallop int
}

func domainSourceLess(a, b domainSourceSortItem) bool {
	return domainCompareSourceKeys(a.key, b.key) < 0
}
func domainSourceReverse(items []domainSourceSortItem) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
func (s *domainSourceSorter) countRun(start int) int {
	if start+1 == len(s.items) {
		return 1
	}
	n := 2
	descending := domainSourceLess(s.items[start+1], s.items[start])
	for start+n < len(s.items) {
		less := domainSourceLess(s.items[start+n], s.items[start+n-1])
		if less != descending {
			break
		}
		n++
	}
	if descending {
		domainSourceReverse(s.items[start : start+n])
	}
	return n
}
func (s *domainSourceSorter) binarySort(lo, hi, start int) {
	if lo == start {
		start++
	}
	for ; start < hi; start++ {
		pivot := s.items[start]
		left, right := lo, start
		for left < right {
			mid := left + (right-left)/2
			if domainSourceLess(pivot, s.items[mid]) {
				right = mid
			} else {
				left = mid + 1
			}
		}
		copy(s.items[left+1:start+1], s.items[left:start])
		s.items[left] = pivot
	}
}
func domainSourcePower(start, n1, n2, n int) int {
	// uint avoids signed overflow in midpoint doubling. Actual slice limits
	// keep these coordinates bounded; no floating point approximation.
	a, b, size := uint(start)*2+uint(n1), uint(start)*2+uint(n1)*2+uint(n2), uint(n)
	for power := 1; ; power++ {
		if a >= size {
			a -= size
			b -= size
		} else if b >= size {
			return power
		}
		a <<= 1
		b <<= 1
	}
}

func domainSourceGallopLeft(key domainSourceSortItem, items []domainSourceSortItem, hint int) int {
	last, offset := 0, 1
	if domainSourceLess(items[hint], key) {
		limit := len(items) - hint
		for offset < limit && domainSourceLess(items[hint+offset], key) {
			last = offset
			offset = domainSourceGallopStep(offset, limit)
		}
		offset = min(offset, limit)
		last += hint
		offset += hint
	} else {
		limit := hint + 1
		for offset < limit && !domainSourceLess(items[hint-offset], key) {
			last = offset
			offset = domainSourceGallopStep(offset, limit)
		}
		offset = min(offset, limit)
		last, offset = hint-offset, hint-last
	}
	last++
	for last < offset {
		mid := last + (offset-last)/2
		if domainSourceLess(items[mid], key) {
			last = mid + 1
		} else {
			offset = mid
		}
	}
	return offset
}
func domainSourceGallopRight(key domainSourceSortItem, items []domainSourceSortItem, hint int) int {
	last, offset := 0, 1
	if domainSourceLess(key, items[hint]) {
		limit := hint + 1
		for offset < limit && domainSourceLess(key, items[hint-offset]) {
			last = offset
			offset = domainSourceGallopStep(offset, limit)
		}
		offset = min(offset, limit)
		last, offset = hint-offset, hint-last
	} else {
		limit := len(items) - hint
		for offset < limit && !domainSourceLess(key, items[hint+offset]) {
			last = offset
			offset = domainSourceGallopStep(offset, limit)
		}
		offset = min(offset, limit)
		last += hint
		offset += hint
	}
	last++
	for last < offset {
		mid := last + (offset-last)/2
		if domainSourceLess(key, items[mid]) {
			offset = mid
		} else {
			last = mid + 1
		}
	}
	return offset
}
func domainSourceGallopStep(offset, limit int) int {
	if offset > (limit-1)/2 {
		return limit
	}
	return offset*2 + 1
}

func (s *domainSourceSorter) mergeAt(i int) {
	a, b := s.pending[i], s.pending[i+1]
	s.pending[i].length = a.length + b.length
	copy(s.pending[i+1:], s.pending[i+2:])
	s.pending = s.pending[:len(s.pending)-1]
	trim := domainSourceGallopRight(s.items[b.start], s.items[a.start:a.start+a.length], 0)
	a.start += trim
	a.length -= trim
	if a.length == 0 {
		return
	}
	b.length = domainSourceGallopLeft(s.items[a.start+a.length-1], s.items[b.start:b.start+b.length], b.length-1)
	if b.length == 0 {
		return
	}
	if a.length <= b.length {
		s.mergeLow(a.start, a.length, b.start, b.length)
	} else {
		s.mergeHigh(a.start, a.length, b.start, b.length)
	}
}

func (s *domainSourceSorter) mergeLow(baseA, na, baseB, nb int) {
	temp := append([]domainSourceSortItem(nil), s.items[baseA:baseA+na]...)
	a, b, dest := 0, baseB, baseA
	s.items[dest] = s.items[b]
	dest++
	b++
	nb--
	if nb == 0 {
		copy(s.items[dest:dest+na], temp[a:a+na])
		return
	}
	if na == 1 {
		copy(s.items[dest:dest+nb], s.items[b:b+nb])
		s.items[dest+nb] = temp[a]
		return
	}
	minGallop := s.minGallop
	for {
		acount, bcount := 0, 0
		for {
			if domainSourceLess(s.items[b], temp[a]) {
				s.items[dest] = s.items[b]
				dest++
				b++
				bcount++
				acount = 0
				nb--
				if nb == 0 {
					goto done
				}
				if bcount >= minGallop {
					break
				}
			} else {
				s.items[dest] = temp[a]
				dest++
				a++
				acount++
				bcount = 0
				na--
				if na == 1 {
					goto copyB
				}
				if acount >= minGallop {
					break
				}
			}
		}
		minGallop++
		for {
			if minGallop > 1 {
				minGallop--
			}
			s.minGallop = minGallop
			acount = domainSourceGallopRight(s.items[b], temp[a:a+na], 0)
			if acount != 0 {
				copy(s.items[dest:dest+acount], temp[a:a+acount])
				dest += acount
				a += acount
				na -= acount
				if na == 1 {
					goto copyB
				}
				if na == 0 {
					goto done
				}
			}
			s.items[dest] = s.items[b]
			dest++
			b++
			nb--
			if nb == 0 {
				goto done
			}
			bcount = domainSourceGallopLeft(temp[a], s.items[b:b+nb], 0)
			if bcount != 0 {
				copy(s.items[dest:dest+bcount], s.items[b:b+bcount])
				dest += bcount
				b += bcount
				nb -= bcount
				if nb == 0 {
					goto done
				}
			}
			s.items[dest] = temp[a]
			dest++
			a++
			na--
			if na == 1 {
				goto copyB
			}
			if acount < 7 && bcount < 7 {
				break
			}
		}
		minGallop++
		s.minGallop = minGallop
	}
done:
	copy(s.items[dest:dest+na], temp[a:a+na])
	return
copyB:
	copy(s.items[dest:dest+nb], s.items[b:b+nb])
	s.items[dest+nb] = temp[a]
}

func (s *domainSourceSorter) mergeHigh(baseA, na, baseB, nb int) {
	temp := append([]domainSourceSortItem(nil), s.items[baseB:baseB+nb]...)
	a, b, dest := baseA+na-1, nb-1, baseB+nb-1
	s.items[dest] = s.items[a]
	dest--
	a--
	na--
	if na == 0 {
		copy(s.items[dest-nb+1:dest+1], temp[:nb])
		return
	}
	if nb == 1 {
		copy(s.items[dest-na+1:dest+1], s.items[baseA:baseA+na])
		s.items[dest-na] = temp[b]
		return
	}
	minGallop := s.minGallop
	for {
		acount, bcount := 0, 0
		for {
			if domainSourceLess(temp[b], s.items[a]) {
				s.items[dest] = s.items[a]
				dest--
				a--
				acount++
				bcount = 0
				na--
				if na == 0 {
					goto done
				}
				if acount >= minGallop {
					break
				}
			} else {
				s.items[dest] = temp[b]
				dest--
				b--
				bcount++
				acount = 0
				nb--
				if nb == 1 {
					goto copyA
				}
				if bcount >= minGallop {
					break
				}
			}
		}
		minGallop++
		for {
			if minGallop > 1 {
				minGallop--
			}
			s.minGallop = minGallop
			acount = na - domainSourceGallopRight(temp[b], s.items[baseA:baseA+na], na-1)
			if acount != 0 {
				copy(s.items[dest-acount+1:dest+1], s.items[a-acount+1:a+1])
				dest -= acount
				a -= acount
				na -= acount
				if na == 0 {
					goto done
				}
			}
			s.items[dest] = temp[b]
			dest--
			b--
			nb--
			if nb == 1 {
				goto copyA
			}
			bcount = nb - domainSourceGallopLeft(s.items[a], temp[:nb], nb-1)
			if bcount != 0 {
				copy(s.items[dest-bcount+1:dest+1], temp[b-bcount+1:b+1])
				dest -= bcount
				b -= bcount
				nb -= bcount
				if nb == 1 {
					goto copyA
				}
				if nb == 0 {
					goto done
				}
			}
			s.items[dest] = s.items[a]
			dest--
			a--
			na--
			if na == 0 {
				goto done
			}
			if acount < 7 && bcount < 7 {
				break
			}
		}
		minGallop++
		s.minGallop = minGallop
	}
done:
	copy(s.items[dest-nb+1:dest+1], temp[:nb])
	return
copyA:
	copy(s.items[dest-na+1:dest+1], s.items[baseA:baseA+na])
	s.items[dest-na] = temp[b]
}

// PaginationLimitError means the protective client cap was reached. It is
// never evidence of complete or known-empty history.
type PaginationLimitError struct{ MaxPages int }

func (e *PaginationLimitError) Error() string {
	return "sber: history pagination cap reached; completeness unknown"
}
func (e *PaginationLimitError) SDKError()     {}
func (e *PaginationLimitError) Unwrap() error { return &sdkErrs.APIError{} }

func (a *OperationsAPI) queryOptions(options []OperationsQuery) (OperationsQuery, error) {
	q := DefaultOperationsQuery()
	if len(options) > 1 {
		return q, NewParseError("options")
	}
	if len(options) == 1 {
		q = options[0]
	}
	if q.MaxPages < 1 {
		return q, NewParseError("max_pages")
	}
	p, err := a.pageOptions([]OperationsPageOptions{{Resource: q.Resource, Limit: q.Limit, From: q.From, To: q.To}})
	if err != nil {
		return q, err
	}
	q.From, q.To = p.From, p.To
	if _, err = NewSourceTimeFilter(q.From, q.To); err != nil {
		return q, err
	}
	return q, nil
}

// Iter is lazy and yields each stable ID once in server order. Errors are a
// separate yield, including after partial results; callers must inspect them.
// Breaking the range stops without fetching another page.
func (a *OperationsAPI) Iter(ctx context.Context, options ...OperationsQuery) iter.Seq2[Operation, error] {
	return func(yield func(Operation, error) bool) {
		_, err := a.walk(ctx, options, func(op Operation) bool { return yield(op, nil) })
		if err != nil {
			yield(Operation{}, err)
		}
	}
}
