# Cycle 4 datetime/application repair — source fixes verified; legacy fuzz qualification open

Private frozen overlay only; **not applied, not independently approved, not project/goal closure**. Production ownership: `parsers.go`, `resources_operations.go`, `resources_collection.go`. Financial/models/client/transfers/foundation/strictjson/rental/MCP bytes are outside scope. The original 171-file baseline and prior FAIL evidence remain unchanged.

## Architecture and four reported blockers

- **DATES-C3-001:** `ParseSourceDate` independently mirrors the canonical DATE entry/scanner: UTF-8 byte lengths 7/8/10, ASCII calendar/week fields, source residual-byte behavior. `ParseSourceDateTime` retains its independent DATETIME separator/clock/offset scanner. `SourceDateTime.DateOnly` means original spelling accepted by DATE, not clock-free DATETIME; `ParseSourceDate` returns its separately inferred civil date. `2024W09423` retains Feb26 23:00 DATETIME and Feb29 DATE/full-day bounds. DATE-valid/DATETIME-invalid forms remain valid date-first bounds rather than being excluded.
- **DATES-C3-002:** `NewSourceTimeFilter` adds canonical timezone-stripped Moscow WALL validation before wire-second truncation. Real Page, queryOptions and walk/Collect use it, along with SourceRequestBounds for both body and metadata. `NewTimeFilter` keeps native instant-order validation; `TimeFilter.Contains` retains truthful native instant comparison. The source constructor can retain reversed real instants in an accepted wall window; it never fakes UTC values. Default 120-day windows now freeze the Moscow clock, strip timezone, then subtract civil days (including historical offset seconds); two additional canonical frozen-clock RED cases exposed the former rounded-offset default. Caller native Now values remain unchanged.
- **DATES-C3-003:** `SourceOperationCompare` mirrors source relational order: shared naive Moscow identity compares civil fields, mixed/fixed identities compare truthful instants. Zero is neither-less-nor-greater, **not** Python equality. The corpus demonstrates 228 strict cycles and 2,964 zero-order/non-equality relations. An arbitrary Go sort with this comparator is not source parity. `SortSourceOperations` is a native shallow-copy port of pinned CPython3.12.3 reverse/run/binary insertion/minrun/powersort/merge/gallop scheduling, bound into both List and Collect. A separately compiled/race-built negative control using Go SliceStable plus this exact source comparator mismatches 326 of the 12,662 canonical rows; its full raw outcomes are retained as an intentional control, not a production defect. No global identity registry; Native/OperationSortKey instants and exact offset metadata are unchanged.
- **DATES-C3-004:** bank `_operation_date` has its own exact `_strptime` directive alternatives, space-padded one-digit day, case-insensitive T and asymmetric Unicode Nd fields. Datetime validation rejects invalid calendar/leap seconds; rejected forms retain raw text. Canonical Python3.12.3 Unicode15 differs from Go1.27.1 Unicode17: the bank conversion pins the actual 68 canonical decimal-zero sets and rejects newer native Nd digits rather than widening ISO or bank grammar. Financial Unicode parsing is untouched.

## Preserved contracts

Native RequestBounds keeps padded early years; native ISOBounds keeps real IANA offsets. SourceRequestBounds retains canonical Linux unpadded early years; SourceISOBounds retains the source's second precision/fixed +03:00 wire suffix, explicitly not an instant offset. Source microsecond truncation, exact fractional UTC-offset value storage, UTC Native adapters for fractional offsets, per-value copy metadata, IANA fold=0/gap attachment, destination overflow checks, explicit native bound nanoseconds and inclusive date-only upper seconds remain. No old exported function signature or source value/model layout was removed or changed. Every foreign baseline entry matches its original seal.

History is UNKNOWN / BankCapProven=false without independent coverage proof, including final empty pages; neither sorting nor an empty synthetic response proves availability/completeness.

## Real canonical provenance and accounting

Canonical HEAD record: `984d45ae6ddbf37224e8146df2788e8821108c9e`. The retained source manifest explicitly describes its audited working tree (HEAD plus 22 staged repairs), not HEAD-only bytes. Actual pinned models/_http/resources source bytes and raw helper/method hashes are checked before/after; no Git mutation or SDK import. Actual canonical execution is `/usr/bin/python3` **3.12.3**, interpreter SHA256 e50d468e8b0adfb05733f5b87b3cff34829c4a8c1aea50c865aa8bdfe4bb150f. Isolated original helpers and complete original Page/List/Iter bodies execute only against synthetic offline transport. The default-only clock harness overrides datetime.now, not source predicates or method bodies. Go never invokes Python.

The original AST inventory seal was produced by Python3.14.7. Its actual module/date-helper dumps match under that producer and differ under 3.12.3 despite identical source bytes. Source-segment hashes omit a final newline and are distinct from full raw-helper hashes and AST dumps. All versions/evidence, including an initially mistaken diagnostic, remain separately retained; no seal was rewritten.

Full unfiltered final scalar/application streams retain **2,001 + 20,011 + 17,781 + 55** cases, with zero source-semantic mismatches under the explicit native/source split. Original and complete native output streams remain byte-identical to round3. Native convention differences remain separately counted, not called source-equal. The original complete corpus retains 9,308 valid / 10,703 invalid / 1,929 fractional-offset / 77 parse-valid conversion failures. Legacy request/ISO format differences remain original 6/1,716 and complete 2,724/4,490. New/confirmation native instant-order windows differ in 5 rows each, native sorts in 3 each, intentionally preserved through separate application semantics.

The prior generic differential auditor wrongly assumed a raw DATETIME lies inside its independent DATE-inferred window: 1 new and 37 confirmation flags are retained and explicitly qualified against original source integer instant/bound fields. No source row/oracle/fixture was repaired, filtered or overwritten.

New owned canonical fixtures: 66 independent scanner rows; 1,155 request windows at all 76 observed Moscow transitions (Page/List/Collect/Iter bindings); 12,662 original-source sorting/application rows up to length 2,049 with 30,847 pair relations and complete triple permutations; 3,044 bank grammar rows (1,474 normalized / 1,570 raw fallbacks, including Unicode17-only rejection controls); six frozen default-clock rows. All actual fixture bytes are sealed.

## Gates, retained failures and narrow unresolved ownership

Explicit pinned Go1.27.1; GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=sum.golang.org, GOWORK=off, GOMAXPROCS=2, readonly modules. Scoped race, vet, build, module verification, compiled race-built native artifact execution and direct/API/source bindings are exercised with complete receipts/event streams. Final counts are in final-gates.json; ordinary suites/subtests and fuzz seed suites/seeds are separate.

All four behavioral RED/GREEN slices and prior reviewer counterexamples are retained. The initial `_windows_test.go` was incorrectly OS-suffixed and excluded on Linux; its no-test result is retained as harness evidence, followed by actual RED under a renamed owned file. A new immutability assertion initially compared separate time.Time Location pointers; its original assertion/log are archived and corrected to true instant/civil/offset comparison, with no production change. File-tool standalone lint warnings used the wrong module cwd and are not production REDs; pinned real module gates passed.

**Not all gates are PASS:** five of six bounded fuzz targets pass, including all three new targets. The unchanged foreign `FuzzReviewCycle3SourceDatetimeContracts` fails on `0001010100`: canonical DATE accepts 0001-01-01, DATETIME rejects it, and unchanged source Page dispatches the full date-first window. Its lines 37–38 incorrectly imply invalid DATETIME means invalid DATE-first bounds. The failing fuzz artifact/log and actual original-source witness are retained; a new owned regression verifies full source behavior. Changing that foreign assertion is outside exclusive ownership. This subcase stops for the parent test-contract decision; no source narrowing, hidden exclusions, assertion weakening or all-green claim. Parent owns combined independent review/acceptance.

## Licensing and modification notices

The audited SDK source remains under MIT, copyright (c) 2026 sber-mcp contributors; notice below. Sorting/scanner/strptime algorithm references are pinned CPython v3.12.3 primary source, downloaded public-only and hashed under evidence/cpython-3.12.3. Modified derivative summary: C sort/scanner behavior expressed as native Go value/copy/slice operations with exact microsecond/source bounds; Python object/refcount/error callbacks are not ported; the domain comparator is fixed, not user-supplied. CPython copyright and full PSF/associated license retained below. No Python runtime or full SDK dependency is added.

### Audited SDK MIT notice

MIT License

Copyright (c) 2026 sber-mcp contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


### CPython retained license

A. HISTORY OF THE SOFTWARE
==========================

Python was created in the early 1990s by Guido van Rossum at Stichting
Mathematisch Centrum (CWI, see https://www.cwi.nl) in the Netherlands
as a successor of a language called ABC.  Guido remains Python's
principal author, although it includes many contributions from others.

In 1995, Guido continued his work on Python at the Corporation for
National Research Initiatives (CNRI, see https://www.cnri.reston.va.us)
in Reston, Virginia where he released several versions of the
software.

In May 2000, Guido and the Python core development team moved to
BeOpen.com to form the BeOpen PythonLabs team.  In October of the same
year, the PythonLabs team moved to Digital Creations, which became
Zope Corporation.  In 2001, the Python Software Foundation (PSF, see
https://www.python.org/psf/) was formed, a non-profit organization
created specifically to own Python-related Intellectual Property.
Zope Corporation was a sponsoring member of the PSF.

All Python releases are Open Source (see https://opensource.org for
the Open Source Definition).  Historically, most, but not all, Python
releases have also been GPL-compatible; the table below summarizes
the various releases.

    Release         Derived     Year        Owner       GPL-
                    from                                compatible? (1)

    0.9.0 thru 1.2              1991-1995   CWI         yes
    1.3 thru 1.5.2  1.2         1995-1999   CNRI        yes
    1.6             1.5.2       2000        CNRI        no
    2.0             1.6         2000        BeOpen.com  no
    1.6.1           1.6         2001        CNRI        yes (2)
    2.1             2.0+1.6.1   2001        PSF         no
    2.0.1           2.0+1.6.1   2001        PSF         yes
    2.1.1           2.1+2.0.1   2001        PSF         yes
    2.1.2           2.1.1       2002        PSF         yes
    2.1.3           2.1.2       2002        PSF         yes
    2.2 and above   2.1.1       2001-now    PSF         yes

Footnotes:

(1) GPL-compatible doesn't mean that we're distributing Python under
    the GPL.  All Python licenses, unlike the GPL, let you distribute
    a modified version without making your changes open source.  The
    GPL-compatible licenses make it possible to combine Python with
    other software that is released under the GPL; the others don't.

(2) According to Richard Stallman, 1.6.1 is not GPL-compatible,
    because its license has a choice of law clause.  According to
    CNRI, however, Stallman's lawyer has told CNRI's lawyer that 1.6.1
    is "not incompatible" with the GPL.

Thanks to the many outside volunteers who have worked under Guido's
direction to make these releases possible.


B. TERMS AND CONDITIONS FOR ACCESSING OR OTHERWISE USING PYTHON
===============================================================

Python software and documentation are licensed under the
Python Software Foundation License Version 2.

Starting with Python 3.8.6, examples, recipes, and other code in
the documentation are dual licensed under the PSF License Version 2
and the Zero-Clause BSD license.

Some software incorporated into Python is under different licenses.
The licenses are listed with code falling under that license.


PYTHON SOFTWARE FOUNDATION LICENSE VERSION 2
--------------------------------------------

1. This LICENSE AGREEMENT is between the Python Software Foundation
("PSF"), and the Individual or Organization ("Licensee") accessing and
otherwise using this software ("Python") in source or binary form and
its associated documentation.

2. Subject to the terms and conditions of this License Agreement, PSF hereby
grants Licensee a nonexclusive, royalty-free, world-wide license to reproduce,
analyze, test, perform and/or display publicly, prepare derivative works,
distribute, and otherwise use Python alone or in any derivative version,
provided, however, that PSF's License Agreement and PSF's notice of copyright,
i.e., "Copyright (c) 2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008, 2009, 2010,
2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023 Python Software Foundation;
All Rights Reserved" are retained in Python alone or in any derivative version
prepared by Licensee.

3. In the event Licensee prepares a derivative work that is based on
or incorporates Python or any part thereof, and wants to make
the derivative work available to others as provided herein, then
Licensee hereby agrees to include in any such work a brief summary of
the changes made to Python.

4. PSF is making Python available to Licensee on an "AS IS"
basis.  PSF MAKES NO REPRESENTATIONS OR WARRANTIES, EXPRESS OR
IMPLIED.  BY WAY OF EXAMPLE, BUT NOT LIMITATION, PSF MAKES NO AND
DISCLAIMS ANY REPRESENTATION OR WARRANTY OF MERCHANTABILITY OR FITNESS
FOR ANY PARTICULAR PURPOSE OR THAT THE USE OF PYTHON WILL NOT
INFRINGE ANY THIRD PARTY RIGHTS.

5. PSF SHALL NOT BE LIABLE TO LICENSEE OR ANY OTHER USERS OF PYTHON
FOR ANY INCIDENTAL, SPECIAL, OR CONSEQUENTIAL DAMAGES OR LOSS AS
A RESULT OF MODIFYING, DISTRIBUTING, OR OTHERWISE USING PYTHON,
OR ANY DERIVATIVE THEREOF, EVEN IF ADVISED OF THE POSSIBILITY THEREOF.

6. This License Agreement will automatically terminate upon a material
breach of its terms and conditions.

7. Nothing in this License Agreement shall be deemed to create any
relationship of agency, partnership, or joint venture between PSF and
Licensee.  This License Agreement does not grant permission to use PSF
trademarks or trade name in a trademark sense to endorse or promote
products or services of Licensee, or any third party.

8. By copying, installing or otherwise using Python, Licensee
agrees to be bound by the terms and conditions of this License
Agreement.


BEOPEN.COM LICENSE AGREEMENT FOR PYTHON 2.0
-------------------------------------------

BEOPEN PYTHON OPEN SOURCE LICENSE AGREEMENT VERSION 1

1. This LICENSE AGREEMENT is between BeOpen.com ("BeOpen"), having an
office at 160 Saratoga Avenue, Santa Clara, CA 95051, and the
Individual or Organization ("Licensee") accessing and otherwise using
this software in source or binary form and its associated
documentation ("the Software").

2. Subject to the terms and conditions of this BeOpen Python License
Agreement, BeOpen hereby grants Licensee a non-exclusive,
royalty-free, world-wide license to reproduce, analyze, test, perform
and/or display publicly, prepare derivative works, distribute, and
otherwise use the Software alone or in any derivative version,
provided, however, that the BeOpen Python License is retained in the
Software, alone or in any derivative version prepared by Licensee.

3. BeOpen is making the Software available to Licensee on an "AS IS"
basis.  BEOPEN MAKES NO REPRESENTATIONS OR WARRANTIES, EXPRESS OR
IMPLIED.  BY WAY OF EXAMPLE, BUT NOT LIMITATION, BEOPEN MAKES NO AND
DISCLAIMS ANY REPRESENTATION OR WARRANTY OF MERCHANTABILITY OR FITNESS
FOR ANY PARTICULAR PURPOSE OR THAT THE USE OF THE SOFTWARE WILL NOT
INFRINGE ANY THIRD PARTY RIGHTS.

4. BEOPEN SHALL NOT BE LIABLE TO LICENSEE OR ANY OTHER USERS OF THE
SOFTWARE FOR ANY INCIDENTAL, SPECIAL, OR CONSEQUENTIAL DAMAGES OR LOSS
AS A RESULT OF USING, MODIFYING OR DISTRIBUTING THE SOFTWARE, OR ANY
DERIVATIVE THEREOF, EVEN IF ADVISED OF THE POSSIBILITY THEREOF.

5. This License Agreement will automatically terminate upon a material
breach of its terms and conditions.

6. This License Agreement shall be governed by and interpreted in all
respects by the law of the State of California, excluding conflict of
law provisions.  Nothing in this License Agreement shall be deemed to
create any relationship of agency, partnership, or joint venture
between BeOpen and Licensee.  This License Agreement does not grant
permission to use BeOpen trademarks or trade names in a trademark
sense to endorse or promote products or services of Licensee, or any
third party.  As an exception, the "BeOpen Python" logos available at
http://www.pythonlabs.com/logos.html may be used according to the
permissions granted on that web page.

7. By copying, installing or otherwise using the software, Licensee
agrees to be bound by the terms and conditions of this License
Agreement.


CNRI LICENSE AGREEMENT FOR PYTHON 1.6.1
---------------------------------------

1. This LICENSE AGREEMENT is between the Corporation for National
Research Initiatives, having an office at 1895 Preston White Drive,
Reston, VA 20191 ("CNRI"), and the Individual or Organization
("Licensee") accessing and otherwise using Python 1.6.1 software in
source or binary form and its associated documentation.

2. Subject to the terms and conditions of this License Agreement, CNRI
hereby grants Licensee a nonexclusive, royalty-free, world-wide
license to reproduce, analyze, test, perform and/or display publicly,
prepare derivative works, distribute, and otherwise use Python 1.6.1
alone or in any derivative version, provided, however, that CNRI's
License Agreement and CNRI's notice of copyright, i.e., "Copyright (c)
1995-2001 Corporation for National Research Initiatives; All Rights
Reserved" are retained in Python 1.6.1 alone or in any derivative
version prepared by Licensee.  Alternately, in lieu of CNRI's License
Agreement, Licensee may substitute the following text (omitting the
quotes): "Python 1.6.1 is made available subject to the terms and
conditions in CNRI's License Agreement.  This Agreement together with
Python 1.6.1 may be located on the internet using the following
unique, persistent identifier (known as a handle): 1895.22/1013.  This
Agreement may also be obtained from a proxy server on the internet
using the following URL: http://hdl.handle.net/1895.22/1013".

3. In the event Licensee prepares a derivative work that is based on
or incorporates Python 1.6.1 or any part thereof, and wants to make
the derivative work available to others as provided herein, then
Licensee hereby agrees to include in any such work a brief summary of
the changes made to Python 1.6.1.

4. CNRI is making Python 1.6.1 available to Licensee on an "AS IS"
basis.  CNRI MAKES NO REPRESENTATIONS OR WARRANTIES, EXPRESS OR
IMPLIED.  BY WAY OF EXAMPLE, BUT NOT LIMITATION, CNRI MAKES NO AND
DISCLAIMS ANY REPRESENTATION OR WARRANTY OF MERCHANTABILITY OR FITNESS
FOR ANY PARTICULAR PURPOSE OR THAT THE USE OF PYTHON 1.6.1 WILL NOT
INFRINGE ANY THIRD PARTY RIGHTS.

5. CNRI SHALL NOT BE LIABLE TO LICENSEE OR ANY OTHER USERS OF PYTHON
1.6.1 FOR ANY INCIDENTAL, SPECIAL, OR CONSEQUENTIAL DAMAGES OR LOSS AS
A RESULT OF MODIFYING, DISTRIBUTING, OR OTHERWISE USING PYTHON 1.6.1,
OR ANY DERIVATIVE THEREOF, EVEN IF ADVISED OF THE POSSIBILITY THEREOF.

6. This License Agreement will automatically terminate upon a material
breach of its terms and conditions.

7. This License Agreement shall be governed by the federal
intellectual property law of the United States, including without
limitation the federal copyright law, and, to the extent such
U.S. federal law does not apply, by the law of the Commonwealth of
Virginia, excluding Virginia's conflict of law provisions.
Notwithstanding the foregoing, with regard to derivative works based
on Python 1.6.1 that incorporate non-separable material that was
previously distributed under the GNU General Public License (GPL), the
law of the Commonwealth of Virginia shall govern this License
Agreement only as to issues arising under or with respect to
Paragraphs 4, 5, and 7 of this License Agreement.  Nothing in this
License Agreement shall be deemed to create any relationship of
agency, partnership, or joint venture between CNRI and Licensee.  This
License Agreement does not grant permission to use CNRI trademarks or
trade name in a trademark sense to endorse or promote products or
services of Licensee, or any third party.

8. By clicking on the "ACCEPT" button where indicated, or by copying,
installing or otherwise using Python 1.6.1, Licensee agrees to be
bound by the terms and conditions of this License Agreement.

        ACCEPT


CWI LICENSE AGREEMENT FOR PYTHON 0.9.0 THROUGH 1.2
--------------------------------------------------

Copyright (c) 1991 - 1995, Stichting Mathematisch Centrum Amsterdam,
The Netherlands.  All rights reserved.

Permission to use, copy, modify, and distribute this software and its
documentation for any purpose and without fee is hereby granted,
provided that the above copyright notice appear in all copies and that
both that copyright notice and this permission notice appear in
supporting documentation, and that the name of Stichting Mathematisch
Centrum or CWI not be used in advertising or publicity pertaining to
distribution of the software without specific, written prior
permission.

STICHTING MATHEMATISCH CENTRUM DISCLAIMS ALL WARRANTIES WITH REGARD TO
THIS SOFTWARE, INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS, IN NO EVENT SHALL STICHTING MATHEMATISCH CENTRUM BE LIABLE
FOR ANY SPECIAL, INDIRECT OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

ZERO-CLAUSE BSD LICENSE FOR CODE IN THE PYTHON DOCUMENTATION
----------------------------------------------------------------------

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY
AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM
LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR
OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR
PERFORMANCE OF THIS SOFTWARE.
