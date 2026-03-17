// WaLink — minimal client-side JavaScript
// Most interactivity is handled by htmx + Alpine.js

// Configure htmx defaults
document.addEventListener('DOMContentLoaded', function() {
  // Show a global indicator on long requests
  document.body.addEventListener('htmx:beforeRequest', function(e) {
    document.body.classList.add('htmx-loading');
  });
  document.body.addEventListener('htmx:afterRequest', function(e) {
    document.body.classList.remove('htmx-loading');
  });

  // Handle htmx errors with a toast
  document.body.addEventListener('htmx:responseError', function(e) {
    const msg = e.detail.xhr?.responseText || 'Something went wrong';
    showToast('error', msg);
  });
});

// Simple toast notification (for JS-triggered messages)
function showToast(type, message) {
  const colors = {
    success: 'bg-emerald-50 text-emerald-800 border-emerald-200',
    error: 'bg-red-50 text-red-800 border-red-200',
    info: 'bg-blue-50 text-blue-800 border-blue-200',
  };
  const toast = document.createElement('div');
  toast.className = `fixed top-4 right-4 z-50 px-4 py-3 rounded-lg text-sm border ${colors[type] || colors.info} shadow-lg transition-all duration-300`;
  toast.textContent = message;
  document.body.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(-8px)';
    setTimeout(() => toast.remove(), 300);
  }, 4000);
}
