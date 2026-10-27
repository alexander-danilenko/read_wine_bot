# 0003. The participant's winner mark is the only record of a win

Status: accepted, 2026-10-09.

A win is stored as `giveaway_participants.winner` and `winner_place`, not in a separate `giveaway_winners` table. One place means a reroll, a publish and `/list` can never disagree about who won. Do not add a second store of winners. A participant replaced by a reroll keeps `rerolled_at` and can never win that giveaway again: clearing `winner` alone would let the next reroll pick the same person.
