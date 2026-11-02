<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
const route = useRoute()
const menuOpen = ref(false)
const toggle = ref<HTMLButtonElement>()
watch(() => route.fullPath, () => { menuOpen.value = false })
function closeMenu(event: KeyboardEvent | MouseEvent) {
  if (event instanceof KeyboardEvent && event.key === 'Escape' && menuOpen.value) {
    menuOpen.value = false
    toggle.value?.focus()
  } else if (event instanceof MouseEvent && !(event.target as Element).closest('.site-header')) menuOpen.value = false
}
onMounted(() => { document.addEventListener('keydown', closeMenu); document.addEventListener('click', closeMenu) })
onUnmounted(() => { document.removeEventListener('keydown', closeMenu); document.removeEventListener('click', closeMenu) })
</script>
<template>
  <a class="skip-link" href="#main">跳到主要内容</a>
  <header class="site-header" :class="{ 'is-menu-open': menuOpen }">
    <div class="container header-inner">
      <RouterLink class="brand" to="/" aria-label="师说 Laoshi 首页"><img src="/brand/laoshi-mark.svg" alt="" width="36" height="36"><span class="brand-text"><strong>师说</strong><span>Laoshi</span></span></RouterLink>
      <button ref="toggle" class="menu-toggle" type="button" :aria-expanded="menuOpen" aria-controls="site-nav" @click="menuOpen = !menuOpen"><span class="sr-only">{{ menuOpen ? '关闭菜单' : '打开菜单' }}</span><svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16" fill="none" stroke="currentColor" stroke-width="1.5"/></svg></button>
      <nav id="site-nav" class="site-nav" :class="{ 'is-open': menuOpen }" aria-label="主导航" @click="menuOpen = false">
        <RouterLink to="/#works"><span>01</span>作品</RouterLink><RouterLink to="/#downloads"><span>02</span>下载</RouterLink><RouterLink to="/about/"><span>03</span>关于</RouterLink><a class="nav-github" href="https://github.com/emperorStorm" target="_blank" rel="noopener noreferrer">GitHub ↗</a>
      </nav>
    </div>
  </header>
  <main id="main" tabindex="-1"><RouterView :key="route.path" /></main>
  <footer class="site-footer"><div class="container footer-inner"><div class="footer-brand"><img src="/brand/laoshi-mark.svg" alt="" width="36" height="36"><span><strong>师说</strong><span>Laoshi</span></span></div><p class="footer-tagline">终身求知，以诚对话。</p><nav class="footer-links" aria-label="页脚导航"><RouterLink to="/#works">作品</RouterLink><RouterLink to="/#downloads">下载</RouterLink><RouterLink to="/about/">关于</RouterLink><a href="https://github.com/emperorStorm" target="_blank" rel="noopener noreferrer">GitHub ↗</a><a href="#main">回到顶部 ↑</a></nav><p class="footer-meta"><span>© {{ new Date().getFullYear() }} 师说 Laoshi</span><span>终身求知，以诚对话。</span></p></div></footer>
</template>
