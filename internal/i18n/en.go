package i18n

func english() map[Key]string {
	return map[Key]string{
		Logged: "✅ %s · %s\n📝 %s\n📅 %s",
		Usage: "Send an expense as:\n<note> <amount> [#category]\n\n" +
			"Examples:\ncoffee 25k\nelectricity 1,2jt #bills\n\n" +
			"Add \"yesterday\" or @2026-11-20 for another date.\n\n" +
			"Commands:\n/today /week /month /cycle — summary\n/last [n] — last n expenses (default 10)",
		InvalidDate:    "Invalid date. Use @YYYY-MM-DD, e.g. @2026-11-20.",
		UnknownCommand: "That command isn't available yet. Type /help for help.",
		InternalError:  "Something went wrong, please try again shortly.",

		SpanToday:       "Today",
		SpanWeek:        "This week",
		SpanMonth:       "This month",
		SpanCycle:       "This cycle",
		SummaryHeader:   "📊 %s (%s)",
		SummaryTotal:    "Total: %s · %d transactions",
		SummaryCategory: "• %s: %s (%d)",
		LastHeader:      "🧾 Last %d expenses:",
		LastLine:        "%s · %s · %s · %s",
		NoExpenses:      "No expenses yet.",
	}
}
