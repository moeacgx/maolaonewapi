# Image Edit Extra Parameter Pricing

## Background

Some image edit upstreams charge by more than output size and quality. AtlasCloud
OpenAI `openai/gpt-image-*/edit` accepts `images` with 1 to 10 input images, and
AtlasCloud xAI `xai/grok-imagine-image/edit` accepts `image_urls`. Price preview
tests showed that input image count can change the final upstream cost.

The existing model-level specification pricing and `image.edit` route pricing
can select a base fixed price, but they did not account for this additive input
image cost.

## Design

`ModelPriceVariants` and `ModelRoutePriceVariants` now support an optional
`extra_params` array:

```json
{
  "gpt-image-2": {
    "image.edit": {
      "resolution_enabled": true,
      "quality_enabled": true,
      "rules": [
        {
          "resolution": "1024x1024",
          "quality": "medium",
          "price": 0.12
        }
      ],
      "extra_params": [
        {
          "key": "input_images",
          "base": 1,
          "unit_price": 0.0135
        }
      ]
    }
  }
}
```

Formula:

```text
final_unit_price = matched_base_price + max(input_images - base, 0) * unit_price
```

The existing output count multiplier such as `n` is applied after this unit
price is calculated. A route may define only `extra_params` with both
`resolution_enabled` and `quality_enabled` disabled; in that case the surcharge
is added to the fallback model price or model-level specification price.

## Request Metadata

Image edit requests populate `BillingParams["input_images"]` before pre-consume.
The count is derived from JSON `images`, legacy `image`, `image_urls` in unknown
fields or `extra_fields`, and multipart values/files under `image`, `image[]`,
`images`, `images[]`, `image_urls`, and `image_urls[]`.

This parameter is not a ratio. It does not multiply the whole request price; it
only contributes the configured absolute surcharge.

## AtlasCloud Adapter

AtlasCloud edit forwarding now supports multiple input images:

- OpenAI `openai/gpt-image-*/edit` sends `images: []`.
- xAI `xai/grok-imagine-image/edit` sends `image_urls: []`.
- More than 10 edit input images are rejected before upstream submission.
- Image input aliases are normalized after `extra_fields` merge so an edit
  payload keeps only the field required by the final AtlasCloud upstream model.
- Data URLs and multipart files are still uploaded through AtlasCloud
  `uploadMedia` before being passed to the edit request.

## Frontend

The default pricing sheet and classic model pricing editor expose image edit
route extra parameter rules under per-request pricing. The classic helper and
JSON validators also preserve `extra_params` so existing route pricing JSON is
not lost when model pricing is edited or synchronized.

## Verification

- `go test ./setting/ratio_setting ./relay/helper ./dto ./relay/channel/atlascloud`
- `go test ./controller ./relay/channel/atlascloud ./relay/helper ./setting/ratio_setting`
- `go test ./relay ./relay/channel/task/atlascloud ./dto`
- `web/default`: `bun run i18n:sync`
- `web/default`: `bun run typecheck`
- `web/classic`: `bun run build`
