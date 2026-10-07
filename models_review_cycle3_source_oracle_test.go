package sber

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// Original Python3.12 pure-helper result, not a manufactured native oracle.
// Fixture SHA256: e868968bc8c14c152daa8adba4ed69f13d7382d33524e7e4293eb65d40c216a3
const cycle3OriginalSourceFinancialOracle = `{
  "canonical_commit": "984d45ae6ddbf37224e8146df2788e8821108c9e",
  "source_sha256": {
    "entities.py": "ba423d88a8d7af0790e89bb46b8062eeb386f4005c5ad844a71318566514ac63",
    "models.py": "b3ecad3c154635a18c95ffd6a5cebb0cd92adf15be3ac4e2bf0474d0008d81c3"
  },
  "interpreter": {
    "executable": "/usr/bin/python3.12",
    "version": "3.12.3"
  },
  "records": [
    {
      "kind": "Money",
      "raw": {
        "amount": "4111111111111111.50",
        "currency": "4111111111111111"
      },
      "expected": {
        "amount": "4111111111111111.50",
        "currency": "4111111111111111"
      }
    },
    {
      "kind": "Account",
      "raw": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "kind": "ctaccount"
      },
      "expected": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "kind": "ctaccount"
      }
    },
    {
      "kind": "Card",
      "raw": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "type": "debit",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "is_main": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "balance_source": "",
        "account_id": "4111111111111111",
        "account_balance": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        }
      },
      "expected": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "type": "debit",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "is_main": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "balance_source": "",
        "account_id": "4111111111111111",
        "account_balance": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        }
      }
    },
    {
      "kind": "Resource",
      "raw": {
        "type": "card",
        "id": "4111111111111111"
      },
      "expected": {
        "type": "card",
        "id": "4111111111111111"
      }
    },
    {
      "kind": "Operation",
      "raw": {
        "id": "4111111111111111",
        "date": "2026-10-01",
        "form": "fixture � 😀 PAN •••• 1111",
        "type": "fixture � 😀 PAN •••• 1111",
        "classification_code": "4111111111111111",
        "creation_channel": "fixture � 😀 PAN •••• 1111",
        "state": "fixture � 😀 PAN •••• 1111",
        "state_name": "fixture � 😀 PAN •••• 1111",
        "state_description": "fixture � 😀 PAN •••• 1111",
        "merchant": "fixture � 😀 PAN •••• 1111",
        "description": "fixture � 😀 PAN •••• 1111",
        "amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "billing_amount": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "national_amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "commission": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "tips": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "refusal_reason": "fixture � 😀 PAN •••• 1111",
        "from_resource": {
          "type": "card",
          "id": "4111111111111111"
        },
        "to_resource": {
          "type": "account",
          "id": "4111111111111111"
        },
        "scope_card_ids": [
          "4111111111111111",
          "literal-second"
        ],
        "is_financial": true,
        "is_hidden": true,
        "balance_after": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "balance_after_resource_id": "4111111111111111",
        "balance_after_resource_name": "fixture � 😀 PAN •••• 1111"
      },
      "expected": {
        "id": "4111111111111111",
        "date": "2026-10-01",
        "form": "fixture � 😀 PAN •••• 1111",
        "type": "fixture � 😀 PAN •••• 1111",
        "classification_code": "4111111111111111",
        "creation_channel": "fixture � 😀 PAN •••• 1111",
        "state": "fixture � 😀 PAN •••• 1111",
        "state_name": "fixture � 😀 PAN •••• 1111",
        "state_description": "fixture � 😀 PAN •••• 1111",
        "merchant": "fixture � 😀 PAN •••• 1111",
        "description": "fixture � 😀 PAN •••• 1111",
        "amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "billing_amount": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "national_amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "commission": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "tips": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "refusal_reason": "fixture � 😀 PAN •••• 1111",
        "from_resource": {
          "type": "card",
          "id": "4111111111111111"
        },
        "to_resource": {
          "type": "account",
          "id": "4111111111111111"
        },
        "scope_card_ids": [
          "4111111111111111",
          "literal-second"
        ],
        "is_financial": true,
        "is_hidden": true,
        "balance_after": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "balance_after_resource_id": "4111111111111111",
        "balance_after_resource_name": "fixture � 😀 PAN •••• 1111"
      }
    },
    {
      "kind": "CardLedgerEntry",
      "raw": {
        "operation_id": "4111111111111111",
        "card_id": "4111111111111111",
        "direction": "debit",
        "amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        }
      },
      "expected": {
        "operation_id": "4111111111111111",
        "card_id": "4111111111111111",
        "direction": "debit",
        "amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        }
      }
    },
    {
      "kind": "Products",
      "raw": {
        "accounts": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "kind": "ctaccount"
          }
        ],
        "cards": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "type": "debit",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "is_main": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "balance_source": "",
            "account_id": "4111111111111111",
            "account_balance": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            }
          }
        ]
      },
      "expected": {
        "accounts": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "kind": "ctaccount"
          }
        ],
        "cards": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "type": "debit",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "is_main": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "balance_source": "",
            "account_id": "4111111111111111",
            "account_balance": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            }
          }
        ]
      }
    },
    {
      "kind": "OperationsPage",
      "raw": {
        "operations": [
          {
            "id": "4111111111111111",
            "date": "2026-10-01",
            "form": "fixture � 😀 PAN •••• 1111",
            "type": "fixture � 😀 PAN •••• 1111",
            "classification_code": "4111111111111111",
            "creation_channel": "fixture � 😀 PAN •••• 1111",
            "state": "fixture � 😀 PAN •••• 1111",
            "state_name": "fixture � 😀 PAN •••• 1111",
            "state_description": "fixture � 😀 PAN •••• 1111",
            "merchant": "fixture � 😀 PAN •••• 1111",
            "description": "fixture � 😀 PAN •••• 1111",
            "amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "billing_amount": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "national_amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "commission": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "tips": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "refusal_reason": "fixture � 😀 PAN •••• 1111",
            "from_resource": {
              "type": "card",
              "id": "4111111111111111"
            },
            "to_resource": {
              "type": "account",
              "id": "4111111111111111"
            },
            "scope_card_ids": [
              "4111111111111111",
              "literal-second"
            ],
            "is_financial": true,
            "is_hidden": true,
            "balance_after": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "balance_after_resource_id": "4111111111111111",
            "balance_after_resource_name": "fixture � 😀 PAN •••• 1111"
          }
        ],
        "next_offset": null
      },
      "expected": {
        "operations": [
          {
            "id": "4111111111111111",
            "date": "2026-10-01",
            "form": "fixture � 😀 PAN •••• 1111",
            "type": "fixture � 😀 PAN •••• 1111",
            "classification_code": "4111111111111111",
            "creation_channel": "fixture � 😀 PAN •••• 1111",
            "state": "fixture � 😀 PAN •••• 1111",
            "state_name": "fixture � 😀 PAN •••• 1111",
            "state_description": "fixture � 😀 PAN •••• 1111",
            "merchant": "fixture � 😀 PAN •••• 1111",
            "description": "fixture � 😀 PAN •••• 1111",
            "amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "billing_amount": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "national_amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "commission": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "tips": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "refusal_reason": "fixture � 😀 PAN •••• 1111",
            "from_resource": {
              "type": "card",
              "id": "4111111111111111"
            },
            "to_resource": {
              "type": "account",
              "id": "4111111111111111"
            },
            "scope_card_ids": [
              "4111111111111111",
              "literal-second"
            ],
            "is_financial": true,
            "is_hidden": true,
            "balance_after": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "balance_after_resource_id": "4111111111111111",
            "balance_after_resource_name": "fixture � 😀 PAN •••• 1111"
          }
        ],
        "next_offset": null
      }
    },
    {
      "kind": "OperationDetailField",
      "raw": {
        "name": "fixture � 😀 PAN •••• 1111",
        "type": "money",
        "value": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        }
      },
      "expected": {
        "name": "fixture � 😀 PAN •••• 1111",
        "type": "money",
        "value": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        }
      }
    },
    {
      "kind": "OperationDetail",
      "raw": {
        "uoh_id": "4111111111111111",
        "form": "fixture � 😀 PAN •••• 1111",
        "title": "fixture � 😀 PAN •••• 1111",
        "amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "state": "DONE",
        "state_name": "fixture � 😀 PAN •••• 1111",
        "state_description": "fixture � 😀 PAN •••• 1111",
        "statement_available": true,
        "fields": [
          {
            "name": "fixture � 😀 PAN •••• 1111",
            "type": "money",
            "value": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            }
          },
          {
            "name": "fixture � 😀 PAN •••• 1111",
            "type": "text",
            "value": "fixture � 😀 PAN •••• 1111"
          },
          {
            "name": "",
            "type": "",
            "value": null
          }
        ]
      },
      "expected": {
        "uoh_id": "4111111111111111",
        "form": "fixture � 😀 PAN •••• 1111",
        "title": "fixture � 😀 PAN •••• 1111",
        "amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "state": "DONE",
        "state_name": "fixture � 😀 PAN •••• 1111",
        "state_description": "fixture � 😀 PAN •••• 1111",
        "statement_available": true,
        "fields": [
          {
            "name": "fixture � 😀 PAN •••• 1111",
            "type": "money",
            "value": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            }
          },
          {
            "name": "fixture � 😀 PAN •••• 1111",
            "type": "text",
            "value": "fixture � 😀 PAN •••• 1111"
          },
          {
            "name": "",
            "type": "",
            "value": null
          }
        ]
      }
    },
    {
      "kind": "CardLimits",
      "raw": {
        "purchase": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "available": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "available_total": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        }
      },
      "expected": {
        "purchase": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "available": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "available_total": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        }
      }
    },
    {
      "kind": "CreditInfo",
      "raw": {
        "limit": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "own_sum": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "debt": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "min_payment": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "min_payment_date": "2026-10-01"
      },
      "expected": {
        "limit": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "own_sum": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "debt": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "min_payment": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "min_payment_date": "2026-10-01"
      }
    },
    {
      "kind": "CardInfo",
      "raw": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "state": "OPEN",
        "card_holder": "fixture � 😀 PAN •••• 1111",
        "pay_system_type": "fixture � 😀 PAN •••• 1111",
        "expire_date": "2026-10-07",
        "limits": {
          "purchase": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          },
          "available": {
            "amount": "9007199254740993.0100",
            "currency": "�"
          },
          "available_total": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          }
        },
        "credit": {
          "limit": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          },
          "own_sum": {
            "amount": "9007199254740993.0100",
            "currency": "�"
          },
          "debt": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          },
          "min_payment": {
            "amount": "9007199254740993.0100",
            "currency": "�"
          },
          "min_payment_date": "2026-10-01"
        }
      },
      "expected": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "state": "OPEN",
        "card_holder": "fixture � 😀 PAN •••• 1111",
        "pay_system_type": "fixture � 😀 PAN •••• 1111",
        "expire_date": "2026-10-07",
        "limits": {
          "purchase": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          },
          "available": {
            "amount": "9007199254740993.0100",
            "currency": "�"
          },
          "available_total": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          }
        },
        "credit": {
          "limit": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          },
          "own_sum": {
            "amount": "9007199254740993.0100",
            "currency": "�"
          },
          "debt": {
            "amount": "4111111111111111.50",
            "currency": "4111111111111111"
          },
          "min_payment": {
            "amount": "9007199254740993.0100",
            "currency": "�"
          },
          "min_payment_date": "2026-10-01"
        }
      }
    },
    {
      "kind": "CategoryAmount",
      "raw": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "external_id": "fixture � 😀 PAN •••• 1111",
        "national_amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "visible_amount": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "count_operations": 3
      },
      "expected": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "external_id": "fixture � 😀 PAN •••• 1111",
        "national_amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "visible_amount": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "count_operations": 3
      }
    },
    {
      "kind": "PFMPeriod",
      "raw": {
        "from_": "2026-10-01",
        "to": "2026-10-07",
        "income_type": "fixture � 😀 PAN •••• 1111",
        "national_amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "visible_amount": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "categories": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "external_id": "fixture � 😀 PAN •••• 1111",
            "national_amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "visible_amount": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "count_operations": 3
          }
        ]
      },
      "expected": {
        "from_": "2026-10-01",
        "to": "2026-10-07",
        "income_type": "fixture � 😀 PAN •••• 1111",
        "national_amount": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "visible_amount": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        },
        "categories": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "external_id": "fixture � 😀 PAN •••• 1111",
            "national_amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "visible_amount": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "count_operations": 3
          }
        ]
      }
    },
    {
      "kind": "PFMAmounts",
      "raw": {
        "periods": [
          {
            "from_": "2026-10-01",
            "to": "2026-10-07",
            "income_type": "fixture � 😀 PAN •••• 1111",
            "national_amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "visible_amount": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "categories": [
              {
                "id": "4111111111111111",
                "name": "fixture � 😀 PAN •••• 1111",
                "external_id": "fixture � 😀 PAN •••• 1111",
                "national_amount": {
                  "amount": "4111111111111111.50",
                  "currency": "4111111111111111"
                },
                "visible_amount": {
                  "amount": "9007199254740993.0100",
                  "currency": "�"
                },
                "count_operations": 3
              }
            ]
          }
        ]
      },
      "expected": {
        "periods": [
          {
            "from_": "2026-10-01",
            "to": "2026-10-07",
            "income_type": "fixture � 😀 PAN •••• 1111",
            "national_amount": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "visible_amount": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            },
            "categories": [
              {
                "id": "4111111111111111",
                "name": "fixture � 😀 PAN •••• 1111",
                "external_id": "fixture � 😀 PAN •••• 1111",
                "national_amount": {
                  "amount": "4111111111111111.50",
                  "currency": "4111111111111111"
                },
                "visible_amount": {
                  "amount": "9007199254740993.0100",
                  "currency": "�"
                },
                "count_operations": 3
              }
            ]
          }
        ]
      }
    },
    {
      "kind": "BankAccount",
      "raw": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "kind": "ctaccount"
      },
      "expected": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "kind": "ctaccount"
      }
    },
    {
      "kind": "BankCard",
      "raw": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "type": "debit",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "is_main": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "balance_source": "",
        "account_id": "4111111111111111",
        "account_balance": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        }
      },
      "expected": {
        "id": "4111111111111111",
        "name": "fixture � 😀 PAN •••• 1111",
        "last4": "1111",
        "type": "debit",
        "state": "OPEN",
        "hidden": true,
        "arrested": true,
        "is_main": true,
        "balance": {
          "amount": "4111111111111111.50",
          "currency": "4111111111111111"
        },
        "balance_source": "",
        "account_id": "4111111111111111",
        "account_balance": {
          "amount": "9007199254740993.0100",
          "currency": "�"
        }
      }
    },
    {
      "kind": "BankPortfolioRaw",
      "raw": {
        "accounts": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "kind": "ctaccount"
          }
        ],
        "cards": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "type": "debit",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "is_main": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "balance_source": "",
            "account_id": "4111111111111111",
            "account_balance": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            }
          }
        ]
      },
      "expected": {
        "accounts": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "kind": "ctaccount"
          }
        ],
        "cards": [
          {
            "id": "4111111111111111",
            "name": "fixture � 😀 PAN •••• 1111",
            "last4": "1111",
            "type": "debit",
            "state": "OPEN",
            "hidden": true,
            "arrested": true,
            "is_main": true,
            "balance": {
              "amount": "4111111111111111.50",
              "currency": "4111111111111111"
            },
            "balance_source": "",
            "account_id": "4111111111111111",
            "account_balance": {
              "amount": "9007199254740993.0100",
              "currency": "�"
            }
          }
        ]
      }
    },
    {
      "kind": "AccountDefault",
      "raw": {
        "id": "4111111111111111",
        "name": "",
        "last4": "",
        "state": "",
        "hidden": false,
        "arrested": false,
        "balance": null,
        "kind": "ctaccount"
      },
      "expected": {
        "id": "4111111111111111",
        "name": "",
        "last4": "",
        "state": "",
        "hidden": false,
        "arrested": false,
        "balance": null,
        "kind": "ctaccount"
      }
    },
    {
      "kind": "MoneyScientific",
      "raw": {
        "amount": "1.2300E-1000",
        "currency": "RUB"
      },
      "expected": {
        "amount": "1.2300E-1000",
        "currency": "RUB"
      }
    },
    {
      "kind": "MoneySignedZero",
      "raw": {
        "amount": "-0.00",
        "currency": "RUB"
      },
      "expected": {
        "amount": "-0.00",
        "currency": "RUB"
      }
    }
  ],
  "source_portfolio_boundary": "_jsonable(portfolio.raw), not direct serialization of a non-dataclass portfolio",
  "no_package_import_or_network": true
}
`

func TestModelCycle3OriginalSourceFinancialSnapshotOracle(t *testing.T) {
	data := []byte(cycle3OriginalSourceFinancialOracle)
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != "e868968bc8c14c152daa8adba4ed69f13d7382d33524e7e4293eb65d40c216a3" {
		t.Fatal("original-source fixture bytes changed")
	}
	var fixture struct {
		CanonicalCommit string `json:"canonical_commit"`
		Records         []struct {
			Kind          string `json:"kind"`
			Raw, Expected json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.CanonicalCommit != "984d45ae6ddbf37224e8146df2788e8821108c9e" || len(fixture.Records) != 22 {
		t.Fatal("canonical source/denominator changed")
	}
	for _, row := range fixture.Records {
		t.Run(row.Kind, func(t *testing.T) {
			var value any
			switch row.Kind {
			case "Money", "MoneyScientific", "MoneySignedZero":
				value = new(Money)
			case "Account", "AccountDefault", "BankAccount":
				value = new(Account)
			case "Card", "BankCard":
				value = new(Card)
			case "Resource":
				value = new(Resource)
			case "Operation":
				value = new(Operation)
			case "CardLedgerEntry":
				value = new(CardLedgerEntry)
			case "Products", "BankPortfolioRaw":
				value = new(Products)
			case "OperationsPage":
				value = new(OperationsPage)
			case "OperationDetailField":
				value = new(OperationDetailField)
			case "OperationDetail":
				value = new(OperationDetail)
			case "CardLimits":
				value = new(CardLimits)
			case "CreditInfo":
				value = new(CreditInfo)
			case "CardInfo":
				value = new(CardInfo)
			case "CategoryAmount":
				value = new(CategoryAmount)
			case "PFMPeriod":
				value = new(PFMPeriod)
			case "PFMAmounts":
				value = new(PFMAmounts)
			default:
				t.Fatal("unaccounted original-source financial case")
			}
			if err := json.Unmarshal(row.Raw, value); err != nil {
				t.Fatal(err)
			}
			switch row.Kind {
			case "BankAccount":
				value = NewBankAccount(*value.(*Account), nil, nil)
			case "BankCard":
				value = NewBankCard(*value.(*Card), nil, nil)
			case "BankPortfolioRaw":
				value = NewBankPortfolio(*value.(*Products), nil)
			}
			want := cycle3DecodedJSON(t, row.Expected)
			cycle3AssertGenericJSON(t, value, want)
		})
	}
}
