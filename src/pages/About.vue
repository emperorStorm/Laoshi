<script setup lang="ts">
import AboutSection from '../components/AboutSection.vue'
import { useSeo } from '../seo'
useSeo('关于师说', 'Laoshi 取“老师”之音。学无止境，彼此为师；认真倾听，让理解发生。师说记录持续学习与实践中的软件作品。', '/about/')
</script>
<template><div class="about-page"><div class="container"><h1 class="page-title">关于师说</h1></div><AboutSection /></div></template>
