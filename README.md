# Tool tracker
Prototype to make tracking tools easy, by labelling them with QR codes. Only
requires a phone with QR scanning and emailing capabilities.

The usual shared spreadsheet tool trackers are trivial, but require discipline and knowledge:
1. discipline because you have to remember to update the tool, that you just
   want to use. If you use it first, then you will almost certainly forget. And
   shared responsibility, if someone says they took a tool, while you are
   working nearby, they might assume you will update the tracker, but you might
   be busy and assume they will update the tracker;
2. knowledge of where the tracker is, this is often implicit, and can be
   different for different tools.

The aim with this, is that if I make tracking as simple as possible, it is more
likely to be used properly. And have as minimal dependencies as possible,
ideally only a smartphone, without any special program set up. E-mail and QR
codes work nicely here.

## Deploying
To deploy, you should set up the go program somewhere it can receive mail on
port 25, and also somewhere where it can host webpages, presumably behind a
company VPN to not have the tracker website open to all.

There is an example NixOS deployment on the branch
[treasure-hunt](https://github.com/KoviRobi/tooltracker/tree/treasure-hunt/), along
with some UV mapped origami cubes/waterbombs. The idea was, to get people to
trial the software and find bugs, that I printed some cubes, hid them
somewhere, recorded it in the tracker with a hint in the comment. Then when
people found it, they got some sweets as a reward/incentive, and hid it for the
next person, using their phone to give a hint. I did find a bug this way, turns
out some mail clients base-64 encode plaintext too.
