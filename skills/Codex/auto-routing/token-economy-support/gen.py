#!/usr/bin/env python3
"""Generate a deterministic corpus with a planted, verifiable answer.

12 service modules carry a `retryAttempts` setting; 3 deviate from the
documented default. 12 UI modules are irrelevant noise, present so that a
full-history fork carries material a targeted brief would omit.
"""

import os
import random
import textwrap

ROOT = "/tmp/te-measure/corpus"
DEFAULT_RETRIES = 3
DEVIANTS = {"billing": 7, "notifications": 5, "search": 1}

SERVICES = [
    "billing", "notifications", "search", "identity", "catalog", "shipping",
    "inventory", "pricing", "reviews", "recommendations", "payments", "audit",
]

UI = [
    "AccountPanel", "CartDrawer", "CheckoutStepper", "FilterSidebar",
    "HeroBanner", "ImageGallery", "NavigationBar", "OrderTimeline",
    "PriceTag", "ProductCard", "ReviewStars", "SearchBox",
]

VERBS = ["fetch", "resolve", "normalize", "validate", "persist", "publish",
         "reconcile", "enrich", "dispatch", "archive"]
NOUNS = ["record", "payload", "snapshot", "envelope", "descriptor", "manifest",
         "ledger", "receipt", "bundle", "token"]


def service_module(name: str, rng: random.Random) -> str:
    retries = DEVIANTS.get(name, DEFAULT_RETRIES)
    head = textwrap.dedent(f"""\
        import {{ HttpClient }} from "../lib/http";
        import {{ Logger }} from "../lib/logger";
        import {{ MetricSink }} from "../lib/metrics";
        import {{ CircuitBreaker }} from "../lib/breaker";

        export interface {name.capitalize()}Config {{
          baseUrl: string;
          timeoutMs: number;
          retryAttempts: number;
          backoffFactor: number;
        }}

        export const {name}Config: {name.capitalize()}Config = {{
          baseUrl: process.env.{name.upper()}_URL ?? "http://localhost:8080",
          timeoutMs: {rng.choice([1500, 2000, 2500, 3000, 5000])},
          retryAttempts: {retries},
          backoffFactor: {rng.choice([1.5, 2.0, 2.5])},
        }};

        export class {name.capitalize()}Service {{
          private readonly breaker = new CircuitBreaker({name}Config.retryAttempts);

          constructor(
            private readonly http: HttpClient,
            private readonly log: Logger,
            private readonly metrics: MetricSink,
          ) {{}}
        """)

    body = []
    for i in range(rng.randint(11, 15)):
        verb = rng.choice(VERBS)
        noun = rng.choice(NOUNS)
        body.append(textwrap.dedent(f"""\

              /**
               * {verb.capitalize()} the {name} {noun}. Retries follow the service
               * configuration; the breaker opens after repeated transport faults.
               */
              async {verb}{noun.capitalize()}{i}(id: string): Promise<{noun.capitalize()}{i}> {{
                const started = Date.now();
                return this.breaker.run(async () => {{
                  const response = await this.http.get(
                    `${{{name}Config.baseUrl}}/{noun}/${{id}}`,
                    {{ timeoutMs: {name}Config.timeoutMs }},
                  );
                  if (!response.ok) {{
                    this.log.warn("{name}.{verb}{noun.capitalize()}{i} failed", {{
                      id,
                      status: response.status,
                      elapsedMs: Date.now() - started,
                    }});
                    throw new Error(`{name} {verb} failed with ${{response.status}}`);
                  }}
                  this.metrics.observe("{name}.{verb}.{noun}", Date.now() - started);
                  return response.json() as Promise<{noun.capitalize()}{i}>;
                }});
              }}

              export interface {noun.capitalize()}{i} {{
                id: string;
                revision: number;
                updatedAt: string;
                attributes: Record<string, string | number | boolean>;
              }}
            """))
    return head + "".join(body) + "\n}\n"


def ui_module(name: str, rng: random.Random) -> str:
    head = textwrap.dedent(f"""\
        import React, {{ useCallback, useEffect, useMemo, useState }} from "react";
        import {{ useTheme }} from "../hooks/useTheme";
        import {{ useTelemetry }} from "../hooks/useTelemetry";

        export interface {name}Props {{
          title?: string;
          compact?: boolean;
          onSelect?: (id: string) => void;
        }}
        """)
    body = []
    for i in range(rng.randint(9, 13)):
        body.append(textwrap.dedent(f"""\

            export function {name}Section{i}({{ title, compact, onSelect }}: {name}Props) {{
              const theme = useTheme();
              const telemetry = useTelemetry("{name}Section{i}");
              const [expanded, setExpanded] = useState(!compact);
              const [items, setItems] = useState<string[]>([]);

              const spacing = useMemo(
                () => (compact ? theme.spacing.tight : theme.spacing.roomy),
                [compact, theme],
              );

              const toggle = useCallback(() => {{
                setExpanded((previous) => !previous);
                telemetry.track("toggle", {{ expanded: !expanded }});
              }}, [expanded, telemetry]);

              useEffect(() => {{
                setItems((current) => (current.length ? current : ["a", "b", "c"]));
              }}, []);

              return (
                <section style={{{{ padding: spacing }}}} aria-expanded={{expanded}}>
                  <header onClick={{toggle}}>{{title ?? "{name} {i}"}}</header>
                  {{expanded &&
                    items.map((item) => (
                      <button key={{item}} onClick={{() => onSelect?.(item)}}>
                        {{item}}
                      </button>
                    ))}}
                </section>
              );
            }}
            """))
    return head + "".join(body)


def main() -> None:
    os.makedirs(f"{ROOT}/services", exist_ok=True)
    os.makedirs(f"{ROOT}/ui", exist_ok=True)

    for name in SERVICES:
        rng = random.Random(f"svc-{name}")
        with open(f"{ROOT}/services/{name}.ts", "w") as fh:
            fh.write(service_module(name, rng))

    for name in UI:
        rng = random.Random(f"ui-{name}")
        with open(f"{ROOT}/ui/{name}.tsx", "w") as fh:
            fh.write(ui_module(name, rng))

    with open(f"{ROOT}/README.md", "w") as fh:
        fh.write(textwrap.dedent(f"""\
            # Platform services

            ## Resilience conventions

            Every service module declares its own transport configuration in
            `services/<name>.ts`. The platform-wide documented default for
            `retryAttempts` is **{DEFAULT_RETRIES}**. A service may deviate only with a
            recorded exception; deviations are considered configuration drift and must
            be reported during audits.

            `timeoutMs` and `backoffFactor` are deliberately per-service and are not
            governed by a platform default.
            """))

    total = 0
    for base, _, files in os.walk(ROOT):
        for f in files:
            total += os.path.getsize(os.path.join(base, f))
    print(f"corpus bytes: {total}")
    print(f"planted deviations: {DEVIANTS}")


if __name__ == "__main__":
    main()
