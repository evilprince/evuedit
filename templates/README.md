# Imported thumbnail templates

This folder contains the ten JSON layout templates from `foodthumb-templates-download.zip`. Each template includes YouTube, Instagram, and five Facebook placement canvases: video, feed, square, Reels, and link preview. The `facebook` CLI platform alias selects the Reels canvas.

The CLI embeds these files into the runtime catalog at build time. Rebuild `evuedit` after
changing a template. When an ID exists in both this folder and the base catalog in
[`thumb_templates.json`](../thumb_templates.json), the file in this folder takes
precedence. Thus the imported `street-food.json` replaces the original built-in layout.

Some templates require extra command-line inputs: `food-comparison` needs
`--compare-file`, `food-deal` needs `--price`, and `restaurant-review` needs
`--secondary-text`. Their JSON layout settings are rendered by the current CLI, including
split images, labels, badges, borders, outlines, panels, safe areas, and gradient direction.
