# OpenAI function tools for `evuedit`

Each module directory contains one OpenAI function tool definition in strict JSON format:

- `preset/tools.json`
- `evufilters/tools.json`
- `evuthumb/tools.json`
- `evumotion/tools.json`
- `evuads/tools.json`

Load the JSON array from the module you want to offer to the model and pass its object as a function tool. These definitions help the model produce CLI parameters; they do not execute the CLI.

The `evuai` planner uses a two-step flow. It first sends a short description of each available module with the request so the model can select the best fit. It then sends the same request again with only that module's function schema to prepare the arguments and suggested command. Use `--module` to limit the modules considered in the selection step.

To turn a returned argument object into a command:

1. Start with `evuedit --module <module-name>`.
2. Convert each non-null property to a CLI flag by replacing underscores with hyphens, then pass its value. For example, `input_dir` becomes `--input-dir`.
3. Omit properties whose value is `null`.
4. Honor the mutual-exclusion and conditional requirements in each function's descriptions. In particular, use exactly one of `file` and `input_dir` for batch-capable modules, and exactly one of `headline` and `headlines_file` for `evuthumb`.

The schemas use `strict: true`, list every property in `required`, and set `additionalProperties` to `false`. Optional CLI flags are represented by nullable properties so the tool schema remains strict. Catalog IDs are copied from the current built-in JSON catalogs; update the corresponding tool schema when those catalogs change.
