# Dimensio Channel UI Design

## Goal

Expose the existing backend Dimensio channel type in the default frontend so an administrator can select Dimensio and create or edit a type `59` channel through the standard channel form.

## Scope

The change is frontend-only. Backend channel type `59`, task routing, ARK `/api/v3` endpoints, model mapping, authentication, and billing behavior remain unchanged.

## Channel Type Registration

Add `59: 'Dimensio'` to the frontend channel type registry and display order. This makes Dimensio available in the new-channel selector and gives existing type `59` channels a stable `Dimensio` label instead of `Unknown` or `#59`.

Use an existing neutral provider icon supported by the current icon adapter. No custom image or new icon dependency is required.

## Form Behavior

Add a type `59` channel configuration with:

- Default base URL: `https://jimeng.dimensio.cn`
- API key hint describing a raw Dimensio API key
- Model hint listing the three supported upstream model IDs
- A warning that the channel is task-only and is called through the ARK `/api/v3` task API

The standard channel form continues to handle name, key, groups, models, model mapping, routing priority, weight, auto-ban, and save behavior. Dimensio will not be added to the generic upstream model-fetch set because its task API does not establish a compatible model-list contract.

Administrators must enter the client-facing source model in `models` and map it to one supported upstream model through the existing `model_mapping` field.

## Internationalization

Add the new provider label, field hints, and task-only warning to every locale supported by the default frontend. English source strings remain the translation keys.

## Error Handling

Existing form validation remains authoritative:

- Name, channel type, key, model, and group requirements are unchanged.
- Model mapping must remain valid JSON.
- The backend still validates and stores type `59` through the normal channel API.
- Runtime requests using unsupported Dimensio models or resolutions continue to return the existing adaptor errors.

## Verification

Add deterministic frontend tests that verify:

- Type `59` resolves to the `Dimensio` label.
- Dimensio appears in the channel type options.
- Its default base URL and provider hints are available through the channel type configuration.
- It is not added to generic upstream model fetching.

Run the focused frontend tests, typecheck, lint for changed files, i18n synchronization checks, and a production build. Perform a browser check that the standard new-channel form displays Dimensio and loads its default settings.
