// feedflow Alpine.jsコンポーネントです。オーバーレイ開閉とテーマ切替とキーボードショートカットと自動既読を担います。
// CSPビルドのAlpineで動かすため、テンプレートのインライン式は使わず、ここで登録した
// プロパティとメソッドだけを参照します。script-srcはselfに限定しunsafe-evalを使いません。

// csrfToken metaタグからCSRFトークンを取得します。
function csrfToken() {
  const meta = document.querySelector('meta[name="csrf-token"]');
  return meta ? meta.getAttribute("content") : "";
}

// postAction 状態変更系のアクションをCSRFトークン付きで送信します。
async function postAction(url, params) {
  const body = new URLSearchParams(params || {});
  await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-CSRF-Token": csrfToken(),
    },
    body: body.toString(),
  });
}

// cardActionData クリック起点の要素から所属カードのフィードIDと記事IDを取り出します。
function cardActionData(target) {
  const card = target.closest(".item-card");
  if (!card) {
    return { feedID: "", itemID: "", card: null };
  }
  return {
    feedID: card.getAttribute("data-feed") || "",
    itemID: card.getAttribute("data-item") || "",
    card,
  };
}

// registerFeedflow Alpineの初期化時にfeedflowコンポーネントを登録します。
function registerFeedflow() {
  window.Alpine.data("feedflow", () => ({
    theme: "dark",
    overlayOpen: false,
    activeFeed: "",
    activeItem: "",

    init() {
      const saved = localStorage.getItem("feedflow-theme");
      const initial = document.documentElement.getAttribute("data-theme");
      if (saved === "dark" || saved === "light") {
        this.theme = saved;
      } else if (initial === "dark" || initial === "light") {
        this.theme = initial;
      }
      this.applyTheme();
    },

    get themeLabel() {
      return this.theme === "dark" ? "昼" : "夜";
    },

    applyTheme() {
      document.documentElement.setAttribute("data-theme", this.theme);
    },

    toggleTheme() {
      this.theme = this.theme === "dark" ? "light" : "dark";
      this.applyTheme();
      localStorage.setItem("feedflow-theme", this.theme);
    },

    openOverlay(event) {
      const { feedID, itemID, card } = cardActionData(event.currentTarget);
      this.activeFeed = feedID;
      this.activeItem = itemID;
      this.overlayOpen = true;
      if (card) {
        card.classList.add("is-read");
      }
    },

    closeOverlay() {
      this.overlayOpen = false;
      this.activeFeed = "";
      this.activeItem = "";
    },

    onOverlayScroll(event) {
      const el = event.target;
      const reachedEnd = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
      if (reachedEnd && this.activeItem) {
        postAction(
          "/app/items/" + this.activeFeed + "/" + this.activeItem + "/read",
          { read: "true" }
        );
      }
    },

    focusNextCard(delta) {
      const cards = Array.from(document.querySelectorAll(".item-card"));
      if (cards.length === 0) {
        return;
      }
      const current = document.activeElement.closest(".item-card");
      let index = current ? cards.indexOf(current) : -1;
      index = Math.min(Math.max(index + delta, 0), cards.length - 1);
      const link = cards[index].querySelector(".item-open");
      if (link) {
        link.focus();
      }
    },

    onKey(event) {
      const tag = (event.target.tagName || "").toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select") {
        return;
      }
      if (event.key === "Escape" && this.overlayOpen) {
        this.closeOverlay();
        return;
      }
      if (event.key === "j") {
        this.focusNextCard(1);
        event.preventDefault();
        return;
      }
      if (event.key === "k") {
        this.focusNextCard(-1);
        event.preventDefault();
        return;
      }
      const card = document.activeElement.closest(".item-card");
      if (!card) {
        return;
      }
      const feedID = card.getAttribute("data-feed");
      const itemID = card.getAttribute("data-item");
      if (event.key === "m") {
        postAction("/app/items/" + feedID + "/" + itemID + "/read", { read: "true" });
        card.classList.add("is-read");
        event.preventDefault();
      }
      if (event.key === "s") {
        postAction("/app/items/" + feedID + "/" + itemID + "/star", { starred: "true" });
        event.preventDefault();
      }
    },
  }));
}

document.addEventListener("alpine:init", registerFeedflow);
