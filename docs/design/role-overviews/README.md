# Role overviews: child, adult, parent

Status: approved design reference (2026-10-30) for todo item UI6 in
[`plans/app-control-todo.md`](../../../plans/app-control-todo.md). The images were
rendered from the approved HTML proposal and contain only fictional fixtures (Alex, Sam).
They are English only; Android provides EN/DA resources.

| File | Role |
|---|---|
| [`child.png`](child.png) | Child: time left today, ask for more time, apps running low |
| [`adult.png`](adult.png) | Adult: open-until window, active pact (Ulysses) card, apps running low |
| [`parent.png`](parent.png) | Parent: "Needs you" requests, "Right now" per child |
| [`roles.png`](roles.png) | All three side by side |

## What the design changes

The current overview stacks several cards that repeat the same status. Problems found on
the maintainer's phone:

- An expired request card sits on top of the overview.
- The hotspot card is oversized.
- The same status is explained three times.
- "Time is not counted right now" is ambiguous: it does not say why or what still applies.
- The bottom navigation has no labels.
- Apps are shown as letters instead of their real icons.
- The "Always OK" grid is cramped.

## Decisions

- **One hero answer per role.** The dark hero card answers the role's main question:
  - child: how much time is left today and when the next pause starts;
  - adult: until when apps are open, then what rests and until when;
  - parent: what needs a decision, then what each child is doing now.
- **Status lives in a chip in the top bar** ("Updated 16:29"), not in extra cards.
- **Real app icons**, falling back to a letter only when no icon is available.
- **Visible navigation labels** on every bottom-navigation item. The rendered images show
  icons only; labels are a requirement on top of them.
- **Honest unknown state.** When measurement is unreliable, the hero says so plainly. It
  also says what still applies (for example that rules and downtime are still enforced)
  and gives the reason when one is known. Never show a fabricated countdown.
- **Per-request approval** for the parent. Each request can be approved or opened
  individually; "Review all" opens the list. Expired requests are not shown on the
  overview.
- **Truthful combined statements.** Only claim "apps and hotspot rest" together when the
  active rules actually restrict both. Otherwise name only what applies.
- The "Running low" list shows only apps with a limit that is close to being used up.
  It does not repeat every rule.

## Implementation notes (UI6)

- **Running low threshold (user decision 2026-10-30):** an app is listed when it is
  limited, not blocked, measured and has more than 0 and at most 5 minutes left
  (`AppControl.WARN_BEFORE_MS`, the existing warning time; RB1 budget remaining). This
  deliberately differs from the images, which show 15–33 minutes left as examples.
  Apps with no time left are paused and are not listed.
- Navigation labels shrink to fit instead of being truncated at large text sizes.
- The pact card omits "day 3 of 6": the pact start is not available on the phone.
- The child hero says "Downtime starts at 20:00" instead of "Pauses at 20:00 for the
  night": downtime windows are not always at night, so the wording names the actual rule.
