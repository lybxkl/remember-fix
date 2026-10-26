import '../lib/polyfills'
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import '../styles.css'
import CaptureApp from '../apps/CaptureApp.vue'

createApp(CaptureApp).use(ElementPlus, { locale: zhCn }).mount('#app')
