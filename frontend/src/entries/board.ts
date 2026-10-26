import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import '../styles.css'
import BoardApp from '../apps/BoardApp.vue'

createApp(BoardApp).use(ElementPlus, { locale: zhCn }).mount('#app')
