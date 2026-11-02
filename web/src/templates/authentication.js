// A runtime template fragment; whitespace and bindings belong to the root view.
export default `    <div v-if="!me" class="auth-layout">
        <div class="auth-card login-card">
        <div class="login-logo" aria-hidden="true">✦</div>
        <h1>让团队协作，自然发生。</h1>
        <form @submit.prevent="authenticate">
          <label v-if="mode === 'register'">昵称<input v-model="name" required maxlength="64" placeholder="你的称呼" /></label>
          <label>邮箱<input v-model="email" type="email" required maxlength="254" placeholder="you@example.com" /></label>
          <label>密码<input v-model="password" type="password" required minlength="8" maxlength="72" placeholder="至少 8 位" /></label>
          <button class="primary" :disabled="loading">{{ mode === 'login' ? '登录' : '创建账号' }}</button>
        </form>
        <button class="text-button" @click="mode = mode === 'login' ? 'register' : 'login'">{{ mode === 'login' ? '还没有账号？立即注册' : '已有账号？返回登录' }}</button>
      </div>
    </div>
`
