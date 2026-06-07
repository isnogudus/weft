<script>
  import { i18n, setLang } from '../lib/i18n.svelte.js'
</script>

<div class="row" style="gap:0">
  <button
    class:active={i18n.lang === 'de'}
    onclick={() => setLang('de')}
    style="border-radius:8px 0 0 8px"
    title="Deutsch">DE</button>
  <button
    class:active={i18n.lang === 'en'}
    onclick={() => setLang('en')}
    style="border-radius:0 8px 8px 0; border-left:none"
    title="English">EN</button>
</div>
