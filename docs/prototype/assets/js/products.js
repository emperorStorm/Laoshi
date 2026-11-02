/* Laoshi prototype data.
   This file is loaded as a classic script so the prototype also works from file://. */
(function () {
  const repo = (name) => `https://github.com/emperorStorm/${name}`;
  const releases = (name) => `${repo(name)}/releases`;

  const products = [
    {
      slug: 'coscool',
      nameZh: '课思库',
      nameEn: 'Coscool',
      version: '0.2.3',
      category: 'desktop',
      categoryLabel: '桌面端',
      localFirst: true,
      tagline: '本地优先的桌面题库管理工具',
      summary: '把题库、图片和批注留在本机，按自己的方式整理与检索。',
      features: [
        '本地题库分类树，按科目与章节自由组织',
        '题目录入与图片管理',
        'Konva 批注板，支持在题目上圈画',
        '多条件检索与筛选',
        '单题导出',
        'SQLite 本地数据仓库，图片、备份与导入统一存放'
      ],
      featureTags: ['本地题库', '图片批注', '多条件检索', 'SQLite 存储'],
      platforms: ['Windows', 'macOS'],
      stack: ['Tauri 2', 'Rust', 'SQLite', 'Vue 3', 'TypeScript', 'Ant Design Vue'],
      accent: '#0a766e',
      accentSoft: '#e3f2ef',
      logo: 'assets/brand/coscool.svg',
      logoAlt: '课思库应用图标',
      shots: [],
      links: {
        repo: repo('coscool'),
        releases: releases('coscool'),
        demo: 'https://emperorStorm.github.io/coscool/prototype/'
      },
      download: {
        status: 'available',
        label: '下载',
        url: releases('coscool'),
        note: '前往 GitHub Releases 查看可用版本'
      }
    },
    {
      slug: 'linkwisehub',
      nameZh: '智链中枢',
      nameEn: 'LinkwiseHub',
      version: '1.0.0',
      category: 'service',
      categoryLabel: 'Web 服务',
      localFirst: false,
      tagline: '后端服务与 Web 后台管理系统',
      summary: '把组织、知识与 AI 能力接成一条链，让后台真正可用。',
      features: [
        '组织、角色与用户权限体系',
        'AI 流式问答',
        'MinerU 文档解析',
        '知识库与向量检索',
        'OnlyOffice 预览与编辑',
        '智能外呼'
      ],
      featureTags: ['权限体系', 'AI 问答', '知识库', '文档解析'],
      platforms: ['Web 后台', 'API'],
      stack: ['Java 21', 'Spring Boot 3.5', 'MyBatis', 'MySQL', 'MinIO', 'Qdrant', 'Vue 3'],
      accent: '#102033',
      accentSoft: '#e7edf4',
      logo: 'assets/brand/linkwisehub.svg',
      logoAlt: '智链中枢应用图标',
      shots: [],
      links: {
        repo: repo('LinkwiseHub'),
        releases: releases('LinkwiseHub'),
        demo: null
      },
      download: {
        status: 'source',
        label: '获取源码',
        url: repo('LinkwiseHub'),
        note: '首版为服务端与后台，请从仓库获取源码'
      }
    },
    {
      slug: 'sui-time',
      nameZh: '岁岁时光',
      nameEn: 'sui-time',
      version: '0.6.4',
      category: 'desktop',
      categoryLabel: '桌面端',
      localFirst: true,
      tagline: '本地优先的个人待办与时间规划应用',
      summary: '把待办放回时间轴上，让每天的计划有迹可循。',
      features: [
        '四象限待办与优先级',
        '周计划、月计划与跨日期拖放',
        '日 / 周 / 月 / 年 / 记忆曲线等重复规则',
        '子事项与最多三个提醒',
        '加密备份',
        '应用内更新'
      ],
      featureTags: ['四象限', '周月计划', '重复规则', '本地加密'],
      platforms: ['Windows', 'macOS', 'Android'],
      stack: ['Tauri 2', 'Rust', 'SQLite', 'Vue 3', 'UniApp', 'Argon2'],
      accent: '#2E5A4C',
      accentSoft: '#e5efeb',
      logo: 'assets/brand/sui-time.png',
      logoAlt: '岁岁时光应用图标',
      shots: [
        { src: 'assets/shots/sui-time-about.png', alt: '岁岁时光关于页界面', width: 1200, height: 818 },
        { src: 'assets/shots/sui-time-month.png', alt: '岁岁时光月计划界面', width: 1200, height: 818 }
      ],
      links: {
        repo: repo('sui-time'),
        releases: releases('sui-time'),
        demo: null
      },
      download: {
        status: 'available',
        label: '下载',
        url: releases('sui-time'),
        note: '前往 GitHub Releases 查看可用版本'
      }
    },
    {
      slug: 'suimelody',
      nameZh: '岁音',
      nameEn: 'SuiMelody',
      version: '0.1.4',
      category: 'mobile',
      categoryLabel: '移动端',
      localFirst: true,
      tagline: '本地优先、可跨端同步的音乐播放器',
      summary: '让自己的音乐库跨端流动，安静地听想听的歌。',
      features: [
        '本地歌单与收藏',
        '搜索、播放队列与歌词',
        '多端数据同步与合并',
        '本地音乐导入',
        '插件审核与审计',
        '桌面歌词与迷你模式'
      ],
      featureTags: ['本地曲库', '多端同步', '歌词队列', '插件审计'],
      platforms: ['Windows', 'macOS', 'Android', 'iOS', 'H5', '微信小程序', 'Web 管理端'],
      stack: ['Tauri 2', 'Spring Boot 3.5', 'MySQL', 'UniApp', 'Vue 3'],
      accent: '#11161c',
      accentSoft: '#f3ead9',
      logo: 'assets/brand/suimelody.svg',
      logoAlt: '岁音应用图标',
      shots: [
        { src: 'assets/shots/suimelody-home.png', alt: '岁音桌面端首页界面', width: 1280, height: 1099 },
        { src: 'assets/shots/suimelody-mobile.png', alt: '岁音移动端首页界面', width: 375, height: 812 }
      ],
      links: {
        repo: repo('SuiMelody'),
        releases: releases('SuiMelody'),
        demo: null
      },
      download: {
        status: 'available',
        label: '下载',
        url: releases('SuiMelody'),
        note: '前往 GitHub Releases 查看可用版本'
      }
    },
    {
      slug: 'suitime-kitchen',
      nameZh: '岁岁食光小厨房',
      nameEn: 'suitime-kitchen',
      version: '1.0.0',
      category: 'mobile',
      categoryLabel: '移动端',
      localFirst: false,
      tagline: '厨房、菜谱与每日菜单的多端全栈应用',
      summary: '把厨房里的琐事做成顺手的工具，从菜谱到购物清单。',
      features: [
        '厨房分类管理',
        '菜谱用料与步骤',
        '每日菜单',
        '购物清单',
        '图片上传与 OSS 存储',
        '微信 OpenID 登录'
      ],
      featureTags: ['菜谱管理', '每日菜单', '购物清单', '微信登录'],
      platforms: ['Windows', 'macOS', 'Android', 'iOS', 'H5', '微信小程序', 'Web 后台'],
      stack: ['Spring Boot 3.5', 'MySQL', '阿里云 OSS', 'UniApp', 'Vue 3', 'Tauri'],
      accent: '#478f61',
      accentSoft: '#eaf3ec',
      logo: 'assets/brand/kitchen-mark.svg',
      logoAlt: '岁岁食光小厨房占位标志',
      shots: [
        { src: 'assets/shots/kitchen-admin.png', alt: '岁岁食光小厨房后台界面', width: 1440, height: 900 },
        { src: 'assets/shots/kitchen-mobile.png', alt: '岁岁食光小厨房移动端首页', width: 375, height: 812 }
      ],
      links: {
        repo: repo('suitime-kitchen'),
        releases: releases('suitime-kitchen'),
        demo: null
      },
      download: {
        status: 'soon',
        label: '即将开放',
        url: repo('suitime-kitchen'),
        note: '安装包准备中，可先查看源码'
      }
    },
    {
      slug: 'suivision',
      nameZh: '岁影',
      nameEn: 'SuiVision',
      version: '0.2.1',
      category: 'desktop',
      categoryLabel: '桌面端',
      localFirst: true,
      tagline: '本地优先的桌面端影视资源检索工具',
      summary: '在多个资源源之间，找到想看的那一部。',
      features: [
        '多源聚合搜索',
        '影视实体识别',
        '四层资源池',
        '来源覆盖统计',
        '相关性分组',
        '外部资源跳转'
      ],
      featureTags: ['聚合搜索', '实体识别', '资源池', '来源统计'],
      platforms: ['Windows', 'macOS'],
      stack: ['Tauri 2', 'Rust', 'Vue 3', 'TypeScript', 'Ant Design Vue'],
      accent: '#0a1020',
      accentSoft: '#e6e9ef',
      logo: 'assets/brand/suivision.svg',
      logoAlt: '岁影应用图标',
      shots: [],
      links: {
        repo: repo('SuiVision'),
        releases: releases('SuiVision'),
        demo: null
      },
      download: {
        status: 'available',
        label: '下载',
        url: releases('SuiVision'),
        note: '前往 GitHub Releases 查看可用版本'
      }
    }
  ];

  window.LAOSHI_PRODUCTS = products;
})();
