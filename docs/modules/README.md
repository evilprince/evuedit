# Module documentation

Each CLI tool has a module guide named `docs/modules/<tool>.md`. The guide is the usage reference for that tool and should be updated in the same change as its implementation.

Every module guide should describe:

- What the tool does and when to use it
- Full command syntax, flags, defaults, and copyable examples
- Input and output requirements, including fixed output location and overwrite behavior
- Processing details that affect results, compatibility, quality, or performance
- Validation failures and the messages users can expect
- Test cases that cover the module, linked to the project's test-case reference where useful

## Current modules

- [preset](preset.md) — apply a color preset to portrait YouTube Shorts, Instagram Reels, or Facebook Reels footage and discover presets by use-case tags.
- [evufilters](evufilters.md) — apply JSON-catalogued filters to JPEG, PNG, or WebP images, individually or as a complete variation set.
- [evuthumb](evuthumb.md) — create deterministic YouTube, Instagram, or Facebook placement images from JSON templates and one or more headlines.
- [evumotion](evumotion.md) — create template-driven animated short videos from a still image, headline, and optional transparent subject layer.
