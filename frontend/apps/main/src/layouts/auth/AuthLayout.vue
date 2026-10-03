<template>
  <div class="min-h-screen flex flex-col bg-secondary">
    <main class="flex-1 flex items-start justify-center pt-[15vh] p-4">
      <div class="w-full max-w-sm">
        <slot></slot>
      </div>
    </main>
    <div class="flex justify-center mb-6">
      <span
        class="text-xs text-center text-muted-foreground"
      >
        Powered by ConectDesk
      </span>
    </div>
  </div>
</template>

<script setup></script>
