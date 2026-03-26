import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"","description":"","frontmatter":{},"headers":[],"relativePath":"DIRMAP.md","filePath":"DIRMAP.md"}');
const _sfc_main = { name: "DIRMAP.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h2 id="docs-site" tabindex="-1">docs-site <a class="header-anchor" href="#docs-site" aria-label="Permalink to &quot;docs-site&quot;">​</a></h2><table tabindex="0"><thead><tr><th>File</th><th>Summary</th></tr></thead></table><table tabindex="0"><thead><tr><th>Directory</th><th>Summary</th></tr></thead><tbody><tr><td>.vitepress</td><td>—</td></tr></tbody></table></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("DIRMAP.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const DIRMAP = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  DIRMAP as default
};
