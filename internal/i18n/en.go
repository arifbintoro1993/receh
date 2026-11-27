package i18n

func english() map[Key]string {
	return map[Key]string{
		Logged: "✅ %s · %s\n📝 %s\n📅 %s",
		Usage: "Send an expense as:\n<note> <amount> [#category]\n\n" +
			"Examples:\ncoffee 25k\nelectricity 1,2jt #bills\n\n" +
			"Add \"yesterday\" or @2026-11-20 for another date.",
		InvalidDate:    "Invalid date. Use @YYYY-MM-DD, e.g. @2026-11-20.",
		UnknownCommand: "That command isn't available yet. Type /help for help.",
		InternalError:  "Couldn't save that, please try again shortly.",
	}
}
