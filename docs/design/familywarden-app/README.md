# FamilyWarden Android design handoff

These are the supplied design references, **not implemented Android sources**:

- [Design canvas](design.html): original self-contained export with bundled-font licenses.
- [UI specification](UI-SPEC.md): supplied specification, preserved for review.
- `FamilyWardenUiState.kt`: supplied state-model draft, outside the app's source sets.

Read the [review and required corrections](REVIEW.md) before adopting the draft.
Its opt-in review suite passes 19 new cases alongside 36 existing Android unit tests.

The code and specification are design proposals, not authoritative security policy.
Implementation must follow the [DNS/product plan](../../../plans/dns-filtering.md),
including later decisions on authorization, Android lifecycle, and English/Danish resources.
Design boards are English only; runtime translation is still required.

The existing app, package ID, Device Owner enrollment, and production configuration
have not been changed by archiving these files. Use the accompanying review findings
before adopting any state derivations or enrollment/removal copy.
