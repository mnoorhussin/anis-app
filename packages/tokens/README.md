# Dar design tokens

Ported from [`anis-chat/src/styles/global.css`](https://github.com/mnoorhussin/anis-chat/blob/b8ce1ade82cac12b5693bd5c93fc93dc7154cdd2/src/styles/global.css), with product requirements from `docs/DESIGN-DIRECTION.md` §§2.3, 3 and 5 at the same revision.

The stylesheet wins where the guide's illustrative radii/type values differ. Both `src/theme.css` and `src/tokens.ts` are maintained explicitly. The local sync test checks all shared declarations, not just a few brand colors, including explicit/system dark themes, shadows, type scale, font stacks and radii. It needs no network access; it does not claim to track future upstream changes automatically.

Product differences:

- No legacy gradient aliases or decorative glow utilities.
- Status hues from §3.2; light soft fills are 12% status / 88% paper mixed in sRGB. Small status labels use separate contrast-tested foregrounds; dark labels and soft fills are tuned for espresso. This preserves the specified fill hues without using inaccessible small text.
- Latin font stacks include Arabic fallbacks for mixed-script conversations.
- Display faces are reserved for screen titles and large analytics figures (plus the brand lockup). UI and section headings use Plex Arabic / Inter.
- Widget uses system fonts, keeping the font payload out of third-party pages. Its mark and typing indicator share `ijamDots` geometry.
- Custom widget headers prefer cream or ink text. If neither meets 4.5:1, black/white is used without changing the customer's background color.

Amiri and Fraunces font files are copied from that marketing revision; Satoshi is retired.
