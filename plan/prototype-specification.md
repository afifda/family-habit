# Clickable Prototype Specification

Status: **Prototype produced and reviewed on 2026-08-07**

## Golden path

1. Parent unlocks Parent Mode.
2. Parent adds Maya with a fox avatar.
3. Parent creates **Read for 15 minutes**, worth 10 points, every weekday.
4. Parent exits Parent Mode.
5. Maya selects her profile and sees the habit.
6. Maya opens the item and selects **I did it**.
7. The item moves to **Waiting for parent**.
8. Maya switches to the profile picker.
9. Parent unlocks Parent Mode and opens Approvals.
10. Parent selects **Approve · +10 points**.
11. Parent exits; Maya returns and sees the item under Done and her balance increased.

## Required alternate paths

- Child withdraws while pending.
- Parent selects **Not yet** with an optional reason.
- Submission fails due to network loss and can be safely retried.
- A repeated approval click does not add points twice.
- Switching profile locks Parent Mode and browser Back does not restore it.
- Wrong-child and unauthorized URLs show a safe permission/not-found screen.
- Today and Approvals empty states.
- Parent reverses an approval, with a warning that reversal is terminal.

## Interaction rules

- Every mutation has idle, submitting/disabled, success, and failure states.
- Toasts supplement rather than replace durable page feedback.
- Route navigation moves focus to the page heading.
- Removing an approval card moves focus to the next item or queue heading.
- Dialog focus is trapped and returns to the invoking control.
- PIN fields use numeric input with visible and accessible labels.
- All controls specify hover, pressed, focus, disabled, and loading states.
- Loading beyond roughly 300ms displays a skeleton or inline progress indicator.
- Retry never silently duplicates a point-changing mutation.

## Required announcements

- “Completion sent to a parent.”
- “Completion withdrawn.”
- “Approved. 10 points added to Maya.”
- “Could not send completion. Try again.”

## Prototype review matrix

| Requirement | Prototype evidence |
|---|---|
| PR-01 | Parent unlock, lock, and session boundary |
| PR-02 | Add/edit/archive child concept |
| PR-03 | Fast child entry and clear active profile |
| PR-04 | Recurring fields and effective-date behavior |
| PR-05 | One-off variation and neutral overdue state |
| PR-06 | Today groups and accessible feedback |
| PR-07 | Submit, withdraw, approve, reject, and reverse |
| PR-08 | Balance and recent activity update |
| PR-09 | Per-child progress, approval count, and daily/weekly/monthly report navigation |
| PR-10 | Responsive, keyboard, focus, contrast, and reduced-motion behavior |

The prototype passes when core paths require no explanatory narration, profile switching cannot retain parent privileges, and every state change communicates its effect clearly.
