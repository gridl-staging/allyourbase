import { resolveComponent, withCtx, createVNode, useSSRContext } from "vue";
import { ssrRenderAttrs, ssrRenderComponent } from "vue/server-renderer";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"OpenAPI Specification","description":"","frontmatter":{"title":"OpenAPI Specification","layout":"page"},"headers":[],"relativePath":"guide/openapi.md","filePath":"guide/openapi.md"}');
const _sfc_main = { name: "guide/openapi.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  const _component_ClientOnly = resolveComponent("ClientOnly");
  const _component_Redoc = resolveComponent("Redoc");
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="openapi-specification" tabindex="-1">OpenAPI Specification <a class="header-anchor" href="#openapi-specification" aria-label="Permalink to &quot;OpenAPI Specification&quot;">​</a></h1><p>AYB ships two OpenAPI surfaces plus a docs UI:</p><ul><li><code>GET /api/openapi.yaml</code> serves the bundled YAML spec (<code>openapi.Spec</code>) with <code>Content-Type: application/yaml</code> and <code>Cache-Control: public, max-age=3600</code>.</li><li><code>GET /api/openapi.json</code> serves a generated JSON OpenAPI document built from the live schema cache.</li><li><code>GET /api/docs</code> serves Swagger UI HTML wired to <code>/api/openapi.json</code>.</li></ul><h2 id="json-spec-behavior-api-openapi-json" tabindex="-1">JSON spec behavior (<code>/api/openapi.json</code>) <a class="header-anchor" href="#json-spec-behavior-api-openapi-json" aria-label="Permalink to &quot;JSON spec behavior (\`/api/openapi.json\`)&quot;">​</a></h2><ul><li>Returns <code>503</code> with <code>schema cache not ready</code> when schema cache data is not available.</li><li>Returns <code>200</code> JSON with <code>ETag</code> and <code>Cache-Control: public, max-age=60</code> when available.</li><li>Honors <code>If-None-Match</code>; matching ETag returns <code>304 Not Modified</code>.</li><li>Regenerates the JSON spec when schema cache <code>BuiltAt</code> changes.</li></ul><h2 id="docs-ui-behavior-api-docs" tabindex="-1">Docs UI behavior (<code>/api/docs</code>) <a class="header-anchor" href="#docs-ui-behavior-api-docs" aria-label="Permalink to &quot;Docs UI behavior (\`/api/docs\`)&quot;">​</a></h2><ul><li>Returns HTML (Swagger UI assets from <code>swagger-ui-dist@5</code> CDN).</li><li>UI loads the JSON surface at <code>/api/openapi.json</code>.</li></ul>`);
  _push(ssrRenderComponent(_component_ClientOnly, null, {
    default: withCtx((_, _push2, _parent2, _scopeId) => {
      if (_push2) {
        _push2(ssrRenderComponent(_component_Redoc, null, null, _parent2, _scopeId));
      } else {
        return [
          createVNode(_component_Redoc)
        ];
      }
    }),
    _: 1
  }, _parent));
  _push(`</div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("guide/openapi.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const openapi = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  openapi as default
};
