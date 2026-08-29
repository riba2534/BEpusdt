<template>
  <a-layout-footer class="footer">
    <div class="footer_title">
      <span class="primary-copyright">{{ currentYear }} © 收款网关. All rights reserved.</span>
    </div>
  </a-layout-footer>
</template>

<script setup lang="ts">
const currentYear = ref(new Date().getFullYear());
</script>

<style lang="scss" scoped>
.footer {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  height: 30px;
  border-top: $border-1 solid $color-border-2;
  padding: 0 16px;
}

.footer_title {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;

  .primary-copyright {
    color: $color-text-2;
    font-weight: 500;
    text-align: center;
  }
}
</style>
