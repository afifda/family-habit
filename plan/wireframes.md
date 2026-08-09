# Low-Fidelity Wireframes

These wireframes define hierarchy and behavior, not final visual polish.

## Shared profile picker — mobile

```text
┌──────────────────────────────┐
│ Habit Home                   │
│ Who is using Habit Home?     │
│                              │
│ ┌────────────┐ ┌────────────┐│
│ │     🦊     │ │     🐼     ││
│ │    Maya    │ │    Leo     ││
│ └────────────┘ └────────────┘│
│                              │
│ ──────────────────────────── │
│ [ 🔒 Parent Mode ]           │
└──────────────────────────────┘
```

The full avatar card is interactive and labelled. Optional child PIN opens a numeric sheet. Parent Mode is visually separated. Loading uses skeleton cards; errors keep a retry action visible.

## Child Today — mobile

```text
┌──────────────────────────────┐
│ [🦊] Maya          [Switch]  │
│ Friday, 7 August             │
│ 2 of 4 done today            │
│                              │
│ TO DO · 2                    │
│ ┌──────────────────────────┐ │
│ │ 🪥 Brush teeth           │ │
│ │ Daily habit · 5 points   │ │
│ │              [I did it]  │ │
│ └──────────────────────────┘ │
│ ┌──────────────────────────┐ │
│ │ 🧺 Put clothes away      │ │
│ │ Still to do · Due Thu    │ │
│ │ 8 points     [I did it]  │ │
│ └──────────────────────────┘ │
│                              │
│ WAITING FOR PARENT · 1       │
│ │ 📚 Read · Sent at 08:42  │ │
│                              │
│ DONE · 1                     │
│ │ ✓ Make the bed · +5      │ │
│                              │
│ [Today]        [My points]   │
└──────────────────────────────┘
```

To do stays first. Progress includes text, not only a bar. After submission the item moves to Waiting and announces “Completion sent to a parent.” A failed optimistic update restores the item with Retry.

## Child task detail — mobile

```text
┌──────────────────────────────┐
│ [‹ Today]                    │
│             🪥               │
│ Brush teeth                  │
│ Daily habit                  │
│ Brush for two minutes.       │
│ Today · 5 points             │
│                              │
│ [         I did it         ] │
└──────────────────────────────┘
```

Pending replaces the action with **Waiting for a parent** and **Undo — I’m not finished**. Approved shows **Nice work!** and the awarded points. Rejected returns to To do with **Ready to try again** and an optional note.

## Parent dashboard — mobile

```text
┌──────────────────────────────┐
│ Parent Mode          [Exit]  │
│ Friday, 7 August             │
│ ┌──────────────────────────┐ │
│ │ 3 waiting for approval   │ │
│ │ [Review now]             │ │
│ └──────────────────────────┘ │
│ TODAY                        │
│ │ 🦊 Maya · 2/4 · 1 waiting│ │
│ │ 🐼 Leo  · 3/3 · reviewed │ │
│                              │
│ [+ Add habit or task]        │
│ Home  Approvals  Tasks  More │
└──────────────────────────────┘
```

## Approval queue — mobile

```text
┌──────────────────────────────┐
│ [‹] Approvals · 3 waiting    │
│ [All] [Maya 2] [Leo 1]      │
│ ┌──────────────────────────┐ │
│ │ 🦊 Maya                  │ │
│ │ Read for 15 minutes      │ │
│ │ Sent at 08:42 · 10 points│ │
│ │ [Not yet] [Approve · +10]│ │
│ └──────────────────────────┘ │
└──────────────────────────────┘
```

Approval advances focus to the next item. **Not yet** opens an optional child-safe note sheet. No swipe-only actions are used.

## Children and editor — mobile

```text
┌──────────────────────────────┐
│ [‹] Children       [+ Add]   │
│ │ 🦊 Maya · 35 points  [›] │
│ │    2/4 today · PIN off   │
│ │ 🐼 Leo · 42 points   [›] │
│ │    3/3 today · PIN on    │
│ [View archived children]     │
└──────────────────────────────┘

┌──────────────────────────────┐
│ Edit child                   │
│ Avatar              [Change] │
│ Nickname            [Maya  ] │
│ Profile color       [••••••] │
│ Child PIN           [Off ▾]  │
│ [Save child]                 │
└──────────────────────────────┘
```

Archiving confirms that history remains and the child disappears from the picker.

## Assignment form — mobile

```text
┌──────────────────────────────┐
│ [×] New habit                │
│ [Recurring] [One-off]        │
│ Title [Brush teeth         ] │
│ Description [             ]  │
│ Icon/color [🪥] [••••]       │
│ WHO [✓ Maya] [Leo]           │
│ POINTS [−] [5] [+]           │
│ SCHEDULE                     │
│ (•) Every day                │
│ ( ) Selected days            │
│ [M][T][W][T][F][S][S]        │
│ Starts [7 Aug 2026]          │
│ SUMMARY Maya · Daily · 5     │
│ [Save habit]                 │
└──────────────────────────────┘
```

One-off mode uses one child and a due date. Edit mode requires an effective date and states that past history will not change. Validation appears inline and in an error summary.

## Child report — parent mobile

```text
┌──────────────────────────────┐
│ [‹] Maya's report            │
│ [Day] [Week] [Month]         │
│ [‹] 3–9 August 2026      [›] │
│                              │
│ Approved               18/24 │
│ Waiting                     2 │
│ Ready to try                1 │
│ Still to do                 3 │
│ Points earned              85 │
│                              │
│ ACTIVITY                     │
│ Sun  3/4 approved · 15 pts   │
│ Mon  4/4 approved · 20 pts   │
│ Tue  2/4 approved · 10 pts   │
└──────────────────────────────┘
```

The selected period is explicit in text. Reports provide counts as well as any visual summary, use the household timezone, start weeks on Sunday, and expose loading, empty, retry, and partial-period states.

## Tablet and desktop adaptations

- Breakpoints: compact below 768px, tablet 768–1199px, desktop 1200px and above.
- Parent screens use a persistent 240–272px sidebar at wider sizes.
- Dashboard uses two or three child cards per row plus recent activity.
- Approvals use master-detail on desktop and stacked cards on mobile.
- Children use cards on mobile and a labelled data table on desktop.
- Assignment editor uses a form up to 720px wide with a sticky summary alongside it.
- Reports use summary cards plus a readable daily breakdown; desktop may add charts only when the same values remain available as text.
- All content remains usable at 320px width and 200% browser zoom.

## Foundation tokens

- Brand dark `#315B52`, brand `#3E7166`, brand pale `#DDEEE9`.
- Background `#F7F5F0`, surface `#FFFFFF`, text `#202724`, muted text `#5E6964`.
- Focus `#235FCC`; exact combinations must pass contrast testing.
- Type: locally served Atkinson Hyperlegible or a system sans-serif stack.
- Spacing: 4, 8, 12, 16, 24, 32, 48, 64px.
- Card radius 16px; control radius 12px; minimum target 48×48px.
- Motion 120–200ms and reduced under `prefers-reduced-motion`.
