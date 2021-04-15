<template>
  <div class="header">
    <div class="container-fluid">
      <div class="mobile-header">
        <a href="" class="navbar-toggle" @click.prevent="toggleMenu">
          <div class="icon-bar"></div>
          <div class="icon-bar"></div>
          <div class="icon-bar"></div>
        </a>
        <div id="team" class="team-name">{{ team.name }}</div>
      </div>
      <ul class="header-nav">
        <li v-if="team"><a :href="'http://'+team.domain+'.slack.com/'" target="_blank">Open Slack</a></li>
      </ul>
      <div class="promo">
        <a href="https://slack-archives.org" target="_blank">
          <img src="../assets/logo.png" alt="SlackArchives.org" class="logo logo-md">
          <img src="../assets/logo-symbol.png" alt="SlackArchives.org" class="logo logo-xs">
        </a>
      </div>
    </div>
  </div>
</template>

<script>
  export default {
    props: ['team'],
    data () {
      return {
        msg: '',
      }
    },
    methods: {
      toggleMenu () {
        this.$emit('toggleMenu')
      }
    }
  }
</script>
