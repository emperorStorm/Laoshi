<script setup lang="ts">
import { useSeo } from '../seo'
useSeo('页面未找到', '这个页面不存在，可以回到师说继续浏览作品。', '/404.html', true)
</script>
<template><section class="container not-found"><p class="section-kicker">404 / NOT FOUND</p><h1>这页还没有写下。</h1><p>页面可能已经移动，或地址输入有误。</p><RouterLink class="btn btn-primary" to="/">回到师说</RouterLink></section></template>
