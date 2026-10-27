# 0005. Races around the end time that the draw accepts

Status: accepted, 2026-10-09.

Two races near a giveaway's end time are known and left open, because closing them would change product rules:

- **An edit can put the button back.** `Editor.check` refuses only finished giveaways: a giveaway can be edited until its winners are recorded. A new button text saved in the up-to-one-minute gap after the end time can re-attach the button after the draw removed it; taps on it answer "ended". Blocking edits once the end time passes changes that rule, so ask the owner first.
- **A join tap racing the draw.** `Service.Join` checks the end time and inserts in a separate statement. A tap that passes the check just before the end can land after the draw read the participants. That user is recorded but cannot win. The window is milliseconds.

A candidate filter error stops that giveaway's draw with nothing saved, and the next minute shuffles again. A filter rule that calls Telegram, such as "still in the channel", must return an error on a failed call, not `false`: otherwise a network blip silently disqualifies people.
