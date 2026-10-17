# Design-contract review tests

Opt-in tests for the **supplied design draft**, not Android application sources.
The original `docs/design/familywarden-app/FamilyWardenUiState.kt` is kept unchanged.
`ReviewedHeadline.kt` is a small review proposal, not an adopted app implementation.

Run using a compatible JDK (17/21), from `android/`:

```sh
./gradlew --no-daemon -I ../test/design-contract/init.gradle testDebugUnitTest
```

The init script adds the draft and review tests to the **unit-test source set only**.
It does not change the application ID, manifest, runtime sources, or packaged APK.
Existing Android unit tests run alongside the review cases. Normal builds do not
include the draft or review helpers.

Cases characterize false-positive headlines, demonstrate minimal proposed guards,
and verify existing pure navigation/pact boundary behavior. Passing means the review
findings are reproducible, not that the supplied draft is production-ready or that
unresolved enrollment/removal permissions have been implemented.

Run this explicitly when changing the design draft; it is not added to CI automatically.
No Android device, router, AdGuard instance, or production DNS setting is contacted.
