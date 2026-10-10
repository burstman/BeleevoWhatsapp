package i18n

// First-contact auto-reply settings (far.*). The greeting a shop sends when a
// customer messages its WhatsApp number for the very first time.
func init() {
	set(En, "far.title", "First-contact auto-reply")
	set(Fr, "far.title", "Réponse auto au premier contact")
	set(Ar, "far.title", "الرد التلقائي عند أول تواصل")

	set(En, "far.subtitle", "Greet a customer automatically the first time they message your WhatsApp number. Because their message opens the 24h service window, this greeting is a normal free-form message and does not need an approved template.")
	set(Fr, "far.subtitle", "Accueillez automatiquement un client la première fois qu'il écrit à votre numéro WhatsApp. Comme son message ouvre la fenêtre de service de 24h, ce message est un message libre normal et n'a pas besoin d'un modèle approuvé.")
	set(Ar, "far.subtitle", "رحّب بالعميل تلقائيًا عند أول رسالة يرسلها إلى رقم واتساب الخاص بك. بما أن رسالته تفتح نافذة الخدمة لمدة 24 ساعة، فهذا الترحيب رسالة عادية غير مقيدة ولا يحتاج إلى قالب معتمد.")

	set(En, "far.enabled", "Send this greeting to new customers")
	set(Fr, "far.enabled", "Envoyer ce message aux nouveaux clients")
	set(Ar, "far.enabled", "أرسل هذا الترحيب للعملاء الجدد")

	set(En, "far.kind", "Message type")
	set(Fr, "far.kind", "Type de message")
	set(Ar, "far.kind", "نوع الرسالة")

	set(En, "far.kindText", "Text")
	set(Fr, "far.kindText", "Texte")
	set(Ar, "far.kindText", "نص")

	set(En, "far.kindImage", "Image")
	set(Fr, "far.kindImage", "Image")
	set(Ar, "far.kindImage", "صورة")

	set(En, "far.kindAudio", "Audio")
	set(Fr, "far.kindAudio", "Audio")
	set(Ar, "far.kindAudio", "صوت")

	set(En, "far.textLabel", "Message")
	set(Fr, "far.textLabel", "Message")
	set(Ar, "far.textLabel", "الرسالة")

	set(En, "far.textPlaceholder", "Hello and welcome! How can we help you today?")
	set(Fr, "far.textPlaceholder", "Bonjour et bienvenue ! Comment pouvons-nous vous aider ?")
	set(Ar, "far.textPlaceholder", "مرحبًا بك! كيف يمكننا مساعدتك؟")

	set(En, "far.captionLabel", "Caption (optional)")
	set(Fr, "far.captionLabel", "Légende (facultatif)")
	set(Ar, "far.captionLabel", "تعليق (اختياري)")

	set(En, "far.imageLabel", "Image (JPG, PNG or WEBP, max 5 MB)")
	set(Fr, "far.imageLabel", "Image (JPG, PNG ou WEBP, 5 Mo max)")
	set(Ar, "far.imageLabel", "صورة (JPG أو PNG أو WEBP، بحد أقصى 5 ميغابايت)")

	set(En, "far.audioLabel", "Audio (MP3, M4A, AAC, AMR or OGG, max 16 MB)")
	set(Fr, "far.audioLabel", "Audio (MP3, M4A, AAC, AMR ou OGG, 16 Mo max)")
	set(Ar, "far.audioLabel", "صوت (MP3 أو M4A أو AAC أو AMR أو OGG، بحد أقصى 16 ميغابايت)")

	set(En, "far.chooseFile", "Choose a file")
	set(Fr, "far.chooseFile", "Choisir un fichier")
	set(Ar, "far.chooseFile", "اختر ملفًا")

	set(En, "far.currentFile", "Current file")
	set(Fr, "far.currentFile", "Fichier actuel")
	set(Ar, "far.currentFile", "الملف الحالي")

	set(En, "far.keepCurrent", "Leave empty to keep the current file.")
	set(Fr, "far.keepCurrent", "Laissez vide pour conserver le fichier actuel.")
	set(Ar, "far.keepCurrent", "اتركه فارغًا للإبقاء على الملف الحالي.")

	set(En, "far.preview", "Preview")
	set(Fr, "far.preview", "Aperçu")
	set(Ar, "far.preview", "معاينة")

	set(En, "far.save", "Save")
	set(Fr, "far.save", "Enregistrer")
	set(Ar, "far.save", "حفظ")

	set(En, "far.none", "No greeting configured yet.")
	set(Fr, "far.none", "Aucun message d'accueil configuré.")
	set(Ar, "far.none", "لم يتم إعداد أي ترحيب بعد.")

	set(En, "far.flashSaved", "Greeting saved.")
	set(Fr, "far.flashSaved", "Message d'accueil enregistré.")
	set(Ar, "far.flashSaved", "تم حفظ الترحيب.")

	set(En, "far.flashMissingText", "Enter the greeting text.")
	set(Fr, "far.flashMissingText", "Saisissez le texte du message d'accueil.")
	set(Ar, "far.flashMissingText", "أدخل نص الترحيب.")

	set(En, "far.flashMissingFile", "Choose a file for the selected greeting type.")
	set(Fr, "far.flashMissingFile", "Choisissez un fichier pour le type de message sélectionné.")
	set(Ar, "far.flashMissingFile", "اختر ملفًا لنوع الترحيب المحدد.")

	set(En, "far.flashTooLarge", "The file is too large.")
	set(Fr, "far.flashTooLarge", "Le fichier est trop volumineux.")
	set(Ar, "far.flashTooLarge", "الملف كبير جدًا.")

	set(En, "far.flashBadFormat", "WhatsApp does not accept this file format.")
	set(Fr, "far.flashBadFormat", "WhatsApp n'accepte pas ce format de fichier.")
	set(Ar, "far.flashBadFormat", "واتساب لا يقبل هذه الصيغة.")

	set(En, "far.flashError", "Could not save the greeting; try again.")
	set(Fr, "far.flashError", "Impossible d'enregistrer le message d'accueil ; réessayez.")
	set(Ar, "far.flashError", "تعذّر حفظ الترحيب؛ حاول مرة أخرى.")

	set(En, "far.runOnce", "It is only ever sent once per customer, the first time they write to you.")
	set(Fr, "far.runOnce", "Il n'est envoyé qu'une seule fois par client, la première fois qu'il vous écrit.")
	set(Ar, "far.runOnce", "يُرسل مرة واحدة فقط لكل عميل، عند أول رسالة يرسلها إليك.")
}
