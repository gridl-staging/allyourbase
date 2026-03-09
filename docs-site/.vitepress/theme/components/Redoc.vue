<template>
  <div id="redoc-container"></div>
</template>

<script setup>
import { onMounted } from "vue";

onMounted(() => {
  const script = document.createElement("script");
  script.src = "https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js";
  script.onload = () => {
    window.Redoc.init("/openapi.yaml", {
      theme: {
        colors: { primary: { main: "#3451b2" } },
        typography: { fontFamily: "var(--vp-font-family-base)", headings: { fontFamily: "var(--vp-font-family-base)" } },
      },
      hideDownloadButton: false,
      expandResponses: "200",
    }, document.getElementById("redoc-container"));
  };
  script.onerror = () => {
    const el = document.getElementById("redoc-container");
    if (el) el.innerHTML = '<p style="padding:2rem;color:#666">Failed to load API documentation. Please check your network connection and reload.</p>';
  };
  document.head.appendChild(script);
});
</script>

<style scoped>
#redoc-container {
  margin: 0 -24px;
}
</style>
