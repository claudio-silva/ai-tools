#!/usr/bin/env python3
"""Plant a cross-file interaction with a determinable correct answer.

`billing` retries 7 times with a 2s timeout and 2.0 backoff, so a failing
charge can still be in flight well beyond `payments`' 4s idempotency window,
which is what deduplicates charges. That interaction is the intended answer;
neither file states it alone.
"""

import re

ROOT = "/tmp/te-measure/corpus"

billing = open(f"{ROOT}/services/billing.ts").read()
billing = re.sub(r"timeoutMs: \d+,", "timeoutMs: 2000,", billing, count=1)
billing = re.sub(r"backoffFactor: [\d.]+,", "backoffFactor: 2.0,", billing, count=1)
billing = billing.replace(
    "export class BillingService {",
    "// Charges are submitted through the shared payments charge path.\n"
    "export class BillingService {",
    1,
)
open(f"{ROOT}/services/billing.ts", "w").write(billing)

payments = open(f"{ROOT}/services/payments.ts").read()
payments = payments.replace(
    "  retryAttempts: number;",
    "  retryAttempts: number;\n  idempotencyWindowMs: number;",
    1,
)
payments = re.sub(
    r"(  retryAttempts: \d+,\n)",
    r"\1  idempotencyWindowMs: 4000,\n",
    payments,
    count=1,
)
payments = payments.replace(
    "export class PaymentsService {",
    "// Charge deduplication is keyed on the request id and holds only for\n"
    "// idempotencyWindowMs. A retry arriving after the window is treated as a\n"
    "// new charge.\nexport class PaymentsService {",
    1,
)
open(f"{ROOT}/services/payments.ts", "w").write(payments)

print("planted: billing timeout=2000 backoff=2.0 retries=7; payments window=4000ms")
