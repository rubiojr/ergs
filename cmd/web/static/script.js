// Ergs Web UI JavaScript

document.addEventListener("DOMContentLoaded", function () {
  initializeNavigation();
  initializeMetadataToggles();
  initializePagination();
});

// Navigation enhancements
function initializeNavigation() {
  // Add active state to current page nav links
  const currentPath = window.location.pathname;
  const navLinks = document.querySelectorAll("nav a");

  navLinks.forEach((link) => {
    if (link.getAttribute("href") === currentPath) {
      link.style.backgroundColor = "#e9ecef";
      link.style.color = "#495057";
    }
  });

  // Handle back button for search
  window.addEventListener("popstate", function (e) {
    if (e.state && e.state.query) {
      const searchInput = document.getElementById("searchInput");
      if (searchInput) {
        searchInput.value = e.state.query;
      }
    }
  });
}

// Metadata toggle functionality
function initializeMetadataToggles() {
  const metadataDetails = document.querySelectorAll(".block-metadata details");

  metadataDetails.forEach((details) => {
    details.addEventListener("toggle", function () {
      if (this.open) {
        // Smooth expand animation
        const content = this.querySelector(".block-metadata-content");
        if (content) {
          content.style.animation = "fadeIn 0.3s ease";
        }
      }
    });
  });
}

// Global keyboard shortcuts
document.addEventListener("keydown", function (e) {
  // Ctrl/Cmd + K to focus search
  if ((e.ctrlKey || e.metaKey) && e.key === "k") {
    e.preventDefault();
    const searchInput = document.querySelector('input[name="q"]');
    if (searchInput) {
      searchInput.focus();
      searchInput.select();
    }
  }

  // Escape to clear search
  if (e.key === "Escape") {
    const searchInput = document.querySelector('input[name="q"]');
    if (searchInput && document.activeElement === searchInput) {
      searchInput.blur();
    }
  }
});

// Pagination functionality
function initializePagination() {
  // Enhance pagination buttons with loading states
  const paginationButtons = document.querySelectorAll(".pagination-btn");

  paginationButtons.forEach((button) => {
    button.addEventListener("click", function () {
      // Add loading state
      this.style.opacity = "0.7";
      this.style.pointerEvents = "none";
    });
  });

  // Add keyboard navigation for pagination
  document.addEventListener("keydown", function (e) {
    // Only handle pagination keys if not in an input field
    if (
      document.activeElement.tagName === "INPUT" ||
      document.activeElement.tagName === "SELECT" ||
      document.activeElement.tagName === "TEXTAREA"
    ) {
      return;
    }

    const currentPage = getCurrentPageFromURL();

    // Left arrow or P key for previous page
    if (
      (e.key === "ArrowLeft" || e.key.toLowerCase() === "p") &&
      currentPage > 1
    ) {
      e.preventDefault();
      changePage(currentPage - 1);
    }

    // Right arrow or N key for next page
    if (
      (e.key === "ArrowRight" || e.key.toLowerCase() === "n") &&
      hasNextPage()
    ) {
      e.preventDefault();
      changePage(currentPage + 1);
    }
  });
}

// Get current page from URL
function getCurrentPageFromURL() {
  const urlParams = new URLSearchParams(window.location.search);
  const page = urlParams.get("page");
  return page ? parseInt(page) : 1;
}

// Check if there's a next page button
function hasNextPage() {
  const nextButton = document.querySelector(
    '.pagination-btn[href*="page=' + (getCurrentPageFromURL() + 1) + '"]',
  );
  return nextButton !== null;
}

// Enhanced page change function with smooth scrolling
function changePage(page) {
  const url = new URL(window.location);
  url.searchParams.set("page", page);

  // Smooth scroll to top before navigation
  window.scrollTo({ top: 0, behavior: "smooth" });

  // Small delay to allow scroll to complete
  setTimeout(() => {
    window.location.href = url.toString();
  }, 200);
}

// Add quick navigation hints
function addNavigationHints() {
  const pagination = document.querySelector(".pagination-controls");
  if (pagination && !document.getElementById("nav-hints")) {
    const hints = document.createElement("div");
    hints.id = "nav-hints";
    hints.style.cssText = `
            margin-top: 1rem;
            text-align: center;
            font-size: 0.8rem;
            color: #6c757d;
            padding: 0.5rem;
            background: #f8f9fa;
            border-radius: 4px;
            border: 1px solid #e9ecef;
        `;
    hints.innerHTML = `
            <strong>Navigation:</strong>
            Use ← → arrow keys or P/N keys for previous/next page •
            Ctrl+K to search •
            Escape to clear search
        `;
    pagination.appendChild(hints);
  }
}

// Initialize navigation hints after pagination is ready
setTimeout(addNavigationHints, 500);
