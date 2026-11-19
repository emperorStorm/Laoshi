import type { Work } from './types'

  const repo = (name: string) => `https://github.com/emperorStorm/${name}`;
  const releases = (name: string) => `${repo(name)}/releases`;

  export const works: Work[] = [
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
      logo: '/brand/coscool.svg',
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
      logo: '/brand/linkwisehub.svg',
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
      version: '0.7.10',
      category: 'desktop',
      categoryLabel: '桌面端',
      localFirst: true,
      tagline: '本地优先的个人待办与时间规划应用',
      summary: '把每天的事项放回时间轴：三视图联动、跨日期拖放，计划、重复事项与纪念日都留在本机。',
      features: [
        '全部事项、我的一周、我的一月三视图，支持跨日期与分类拖放',
        '时间与类型筛选：今日至本月，日程、清单与重复事项',
        '四象限优先级、备注、子事项与最多三个提醒',
        '日、周、月、年、记忆曲线等重复规则，单次实例可独立调整',
        '未完成的一次性逾期待办自动顺延到当天',
        '纪念日：倒计时、生日、节日与周年，支持农历、生肖星座、照片与置顶',
        'Argon2 + AES-256-GCM 加密备份、应用内更新；桌面与安卓账号同步（内测）'
      ],
      featureTags: ['三视图', '跨日拖放', '记忆曲线', '纪念日'],
      platforms: ['Windows', 'macOS', 'Android'],
      stack: ['Tauri 2', 'Rust', 'SQLite', 'Vue 3', 'TypeScript', 'UniApp', 'Argon2'],
      accent: '#2E5A4C',
      accentSoft: '#e5efeb',
      logo: '/brand/sui-time.svg',
      logoAlt: '岁岁时光应用图标',
      shots: [
        { src: '/shots/sui-time-all.png', alt: '岁岁时光全部事项界面', width: 1360, height: 900 },
        { src: '/shots/sui-time-month.png', alt: '岁岁时光我的一月界面', width: 1360, height: 900 },
        { src: '/shots/sui-time-anniversary.png', alt: '岁岁时光纪念日界面', width: 1360, height: 900 }
      ],
      links: {
        repo: repo('sui-time'),
        releases: releases('sui-time'),
        demo: 'https://emperorStorm.github.io/sui-time/'
      },
      download: {
        status: 'available',
        label: '下载',
        url: releases('sui-time'),
        note: '安装包以 GitHub Releases 为准；在线原型为浏览器演示，数据只保存在本地。'
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
      logo: '/brand/suimelody.svg',
      logoAlt: '岁音应用图标',
      shots: [
        { src: '/shots/suimelody-home.png', alt: '岁音桌面端首页界面', width: 1280, height: 1099 },
        { src: '/shots/suimelody-mobile.png', alt: '岁音移动端首页界面', width: 375, height: 812 }
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
      logo: '/brand/kitchen-mark.svg',
      logoAlt: '岁岁食光小厨房占位标志',
      shots: [
        { src: '/shots/kitchen-admin.png', alt: '岁岁食光小厨房后台界面', width: 1440, height: 900 },
        { src: '/shots/kitchen-mobile.png', alt: '岁岁食光小厨房移动端首页', width: 375, height: 812 }
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
      logo: '/brand/suivision.svg',
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
