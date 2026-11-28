package i18n

func indonesian() map[Key]string {
	return map[Key]string{
		Logged: "✅ %s · %s\n📝 %s\n📅 %s",
		Usage: "Kirim pengeluaran dengan format:\n<catatan> <jumlah> [#kategori]\n\n" +
			"Contoh:\nkopi 25k\nlistrik 1,2jt #bills\n\n" +
			"Tambahkan \"kemarin\" atau @2026-11-20 untuk tanggal lain.\n\n" +
			"Perintah:\n/today /week /month /cycle — ringkasan\n/last [n] — n pengeluaran terakhir (default 10)",
		InvalidDate:    "Tanggal tidak valid. Gunakan format @YYYY-MM-DD, contoh @2026-11-20.",
		UnknownCommand: "Perintah belum tersedia. Ketik /help untuk bantuan.",
		InternalError:  "Gagal memproses, coba lagi sebentar lagi.",

		SpanToday:       "Hari ini",
		SpanWeek:        "Minggu ini",
		SpanMonth:       "Bulan ini",
		SpanCycle:       "Siklus ini",
		SummaryHeader:   "📊 %s (%s)",
		SummaryTotal:    "Total: %s · %d transaksi",
		SummaryCategory: "• %s: %s (%d)",
		LastHeader:      "🧾 %d pengeluaran terakhir:",
		LastLine:        "%s · %s · %s · %s",
		NoExpenses:      "Belum ada pengeluaran.",
	}
}
