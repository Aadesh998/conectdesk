package telegram

import "testing"

func TestFormatHTML(t *testing.T) {
	cases := map[string]string{
		`<p>Hello <strong>Asha</strong>, your order is <em>ready</em>.</p><p>Thanks!</p>`: "Hello <b>Asha</b>, your order is <i>ready</i>.\n\nThanks!",
		`<p>Track it <a href="https://acme.example/t?a=1&amp;b=2">here</a></p>`:           `Track it <a href="https://acme.example/t?a=1&amp;b=2">here</a>`,
		`<p><a href="javascript:alert(1)">click</a> now</p>`:                              "click now",
		`<p>1 &lt; 2 &amp; <script>x</script>3 &gt; 2</p>`:                                "1 &lt; 2 &amp; 3 &gt; 2",
		`<ul><li>One</li><li>Two</li></ul><ol><li>First</li><li>Second</li></ol>`:         "• One\n• Two\n\n1. First\n2. Second",
		`<p>Use <code>ORDER42</code><br>next line</p>`:                                    "Use <code>ORDER42</code>\nnext line",
		`<b><i>both</b> after</i>`:                                                        "<b><i>both</i></b> after",
		`<p><u>under</u> <s>gone</s></p><blockquote>quoted</blockquote>`:                  "<u>under</u> <s>gone</s>\n\n<blockquote>quoted</blockquote>",
		`<strong>never closed`:                                                            "<b>never closed</b>",
		`<p>nbsp&nbsp;here</p><img src="x.png">`:                                          "nbsp here",
	}
	for in, want := range cases {
		if got := FormatHTML(in); got != want {
			t.Errorf("FormatHTML(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}
