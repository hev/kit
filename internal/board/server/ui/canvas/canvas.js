// hev board canvas: the answer bar. Load it with
//   <script src="/c/_/canvas.js" defer></script>
// Any input, select or textarea with a name, inside an element with class
// cv-label (or data-answer), is an answer. When the page has any, a bar at
// the bottom sends them all, as JSON, as a reply on the canvas's own board
// thread, where the agent that published it reads them with
// `hev board read <id>`. Nothing is stored in the page itself.
(function () {
  var m = location.pathname.match(/^\/c\/(\d+)\//);
  if (!m) return;
  var id = m[1];

  function fields() {
    return Array.prototype.slice.call(
      document.querySelectorAll(".cv-label [name], [data-answer] [name]")
    );
  }

  function answers() {
    var out = {};
    fields().forEach(function (el) {
      var k = el.name;
      if ((el.type === "radio" || el.type === "checkbox") && !el.checked) return;
      var v = el.value;
      if (el.type === "checkbox") {
        (out[k] = out[k] || []).push(v);
      } else if (v !== "") {
        out[k] = v;
      }
    });
    return out;
  }

  window.FactoryCanvas = { id: id, answers: answers };

  document.addEventListener("DOMContentLoaded", function () {
    if (!fields().length || document.querySelector(".cv-answer")) return;
    var bar = document.createElement("form");
    bar.className = "cv-answer";
    bar.method = "post";
    bar.action = "/t/" + id + "/reply";
    bar.target = "_top";
    bar.innerHTML =
      '<span class="count"></span>' +
      '<input name="author" placeholder="your name" size="12" autocomplete="name">' +
      '<input name="note" placeholder="a note (optional)" size="28">' +
      '<input type="hidden" name="body">' +
      '<button class="primary" type="submit">Send answers to the thread</button>';
    document.body.appendChild(bar);
    var count = bar.querySelector(".count");
    var refresh = function () {
      var n = Object.keys(answers()).length;
      count.textContent = n + " answered";
    };
    document.addEventListener("change", refresh);
    document.addEventListener("input", refresh);
    refresh();
    bar.addEventListener("submit", function () {
      var note = bar.elements.note.value.trim();
      bar.elements.body.value =
        (note ? note + "\n\n" : "") +
        "Answers from the canvas:\n\n```json\n" +
        JSON.stringify(answers(), null, 2) +
        "\n```";
    });
  });
})();
