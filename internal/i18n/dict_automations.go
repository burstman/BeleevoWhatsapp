package i18n

// Automation-facing strings: the automation list, the create/edit form, the
// send-history ledger and the held-back suppressions section.
func init() {
	// --- automations list ---
	set(En, "automations.subtitle", "Each automation sends an approved template when an event happens — instant, at a fixed time, or after a delay.")
	set(Fr, "automations.subtitle", "Chaque automatisation envoie un modèle approuvé lorsqu'un événement se produit — immédiatement, à une heure fixe, ou après un délai.")
	set(Ar, "automations.subtitle", "كل أتمتة ترسل قالبًا معتمدًا عند حدوث حدث — فورًا أو في وقت محدد أو بعد تأخير.")

	set(En, "automations.new", "New automation")
	set(Fr, "automations.new", "Nouvelle automatisation")
	set(Ar, "automations.new", "أتمتة جديدة")

	set(En, "automations.none", "No automations yet")
	set(Fr, "automations.none", "Aucune automatisation pour le moment")
	set(Ar, "automations.none", "لا توجد أتمتة بعد")

	set(En, "automations.noneHint", "Create the first one — for example a \"delivered\" event that sends your order_delivered template at 10:00.")
	set(Fr, "automations.noneHint", "Créez la première — par exemple un événement \"livré\" qui envoie votre modèle order_delivered à 10:00.")
	set(Ar, "automations.noneHint", "أنشئ الأولى — على سبيل المثال حدث «تم التسليم» يرسل قالب order_delivered عند 10:00.")

	set(En, "automations.create", "Create an automation")
	set(Fr, "automations.create", "Créer une automatisation")
	set(Ar, "automations.create", "إنشاء أتمتة")

	set(En, "automations.active", "Active")
	set(Fr, "automations.active", "Active")
	set(Ar, "automations.active", "مفعّلة")

	set(En, "automations.paused", "Paused")
	set(Fr, "automations.paused", "En pause")
	set(Ar, "automations.paused", "موقوفة")

	set(En, "automations.ownerShop", "Owner shop")
	set(Fr, "automations.ownerShop", "Boutique propriétaire")
	set(Ar, "automations.ownerShop", "المتجر المالك")

	set(En, "automations.pause", "Pause")
	set(Fr, "automations.pause", "Mettre en pause")
	set(Ar, "automations.pause", "إيقاف")

	set(En, "automations.enable", "Enable")
	set(Fr, "automations.enable", "Activer")
	set(Ar, "automations.enable", "تفعيل")

	set(En, "automations.history", "History")
	set(Fr, "automations.history", "Historique")
	set(Ar, "automations.history", "السجل")

	set(En, "automations.edit", "Edit")
	set(Fr, "automations.edit", "Modifier")
	set(Ar, "automations.edit", "تعديل")

	set(En, "automations.test", "Test")
	set(Fr, "automations.test", "Tester")
	set(Ar, "automations.test", "اختبار")

	set(En, "automations.delete", "Delete")
	set(Fr, "automations.delete", "Supprimer")
	set(Ar, "automations.delete", "حذف")

	set(En, "automations.deleteConfirm", "Delete this automation?")
	set(Fr, "automations.deleteConfirm", "Supprimer cette automatisation ?")
	set(Ar, "automations.deleteConfirm", "هل تريد حذف هذه الأتمتة؟")

	set(En, "automations.type", "Type")
	set(Fr, "automations.type", "Type")
	set(Ar, "automations.type", "النوع")

	set(En, "automations.typeInstant", "Instant")
	set(Fr, "automations.typeInstant", "Immédiate")
	set(Ar, "automations.typeInstant", "فورية")

	set(En, "automations.typeScheduled", "Scheduled")
	set(Fr, "automations.typeScheduled", "Programmée")
	set(Ar, "automations.typeScheduled", "مجدولة")

	set(En, "automations.typeDelayed", "Delayed")
	set(Fr, "automations.typeDelayed", "Différée")
	set(Ar, "automations.typeDelayed", "مؤجلة")

	set(En, "automations.delayDetail", "%d min after the event")
	set(Fr, "automations.delayDetail", "%d min après l'événement")
	set(Ar, "automations.delayDetail", "%d دقيقة بعد الحدث")

	set(En, "automations.allDays", "All days")
	set(Fr, "automations.allDays", "Tous les jours")
	set(Ar, "automations.allDays", "كل الأيام")

	set(En, "automations.trigger", "Trigger")
	set(Fr, "automations.trigger", "Déclencheur")
	set(Ar, "automations.trigger", "المشغّل")

	set(En, "automations.template", "Template")
	set(Fr, "automations.template", "Modèle")
	set(Ar, "automations.template", "القالب")

	set(En, "automations.message", "Message")
	set(Fr, "automations.message", "Message")
	set(Ar, "automations.message", "الرسالة")

	set(En, "automations.deleted", "Deleted")
	set(Fr, "automations.deleted", "Supprimé")
	set(Ar, "automations.deleted", "محذوف")

	set(En, "automations.approved", "Approved")
	set(Fr, "automations.approved", "Approuvé")
	set(Ar, "automations.approved", "معتمد")

	set(En, "automations.testMessage", "Test message")
	set(Fr, "automations.testMessage", "Message de test")
	set(Ar, "automations.testMessage", "رسالة اختبار")

	set(En, "automations.templateLabel", "Template:")
	set(Fr, "automations.templateLabel", "Modèle :")
	set(Ar, "automations.templateLabel", "القالب:")

	set(En, "automations.phoneNumber", "WhatsApp number")
	set(Fr, "automations.phoneNumber", "Numéro WhatsApp")
	set(Ar, "automations.phoneNumber", "رقم واتساب")

	set(En, "automations.phonePlaceholder", "e.g. +21624118849")
	set(Fr, "automations.phonePlaceholder", "ex. +21624118849")
	set(Ar, "automations.phonePlaceholder", "مثال: +21624118849")

	set(En, "automations.testHelp", "Use international format. The recipient must be in your WhatsApp test phone list or have an open 24h conversation.")
	set(Fr, "automations.testHelp", "Utilisez le format international. Le destinataire doit figurer dans votre liste de numéros de test WhatsApp ou disposer d'une conversation ouverte depuis moins de 24 h.")
	set(Ar, "automations.testHelp", "استخدم الصيغة الدولية. يجب أن يكون المستلم ضمن قائمة أرقام اختبار واتساب أو لديه محادثة مفتوحة منذ أقل من 24 ساعة.")

	set(En, "automations.sendTest", "Send test message")
	set(Fr, "automations.sendTest", "Envoyer le message de test")
	set(Ar, "automations.sendTest", "إرسال رسالة الاختبار")

	// --- day names (used in the schedule picker and the history "days" label) ---
	days := []struct {
		key  string
		en   string
		fr   string
		ar   string
	}{{"Mon", "Mon", "Lun", "الإثنين"}, {"Tue", "Tue", "Mar", "الثلاثاء"}, {"Wed", "Wed", "Mer", "الأربعاء"}, {"Thu", "Thu", "Jeu", "الخميس"}, {"Fri", "Fri", "Ven", "الجمعة"}, {"Sat", "Sat", "Sam", "السبت"}, {"Sun", "Sun", "Dim", "الأحد"}}
	for _, d := range days {
		set(En, "automations.day."+d.key, d.en)
		set(Fr, "automations.day."+d.key, d.fr)
		set(Ar, "automations.day."+d.key, d.ar)
	}

	// --- automation form ---
	set(En, "form.editAutomation", "Edit automation")
	set(Fr, "form.editAutomation", "Modifier l'automatisation")
	set(Ar, "form.editAutomation", "تعديل الأتمتة")

	set(En, "form.newAutomation", "New automation")
	set(Fr, "form.newAutomation", "Nouvelle automatisation")
	set(Ar, "form.newAutomation", "أتمتة جديدة")

	set(En, "form.templateHint", "The template is one of your approved templates — its content is edited in the Templates menu.")
	set(Fr, "form.templateHint", "Le modèle est l'un de vos modèles approuvés — son contenu s'édite dans le menu Modèles.")
	set(Ar, "form.templateHint", "القالب هو أحد قوالبك المعتمدة — يُعدَّل محتواه من قائمة القوالب.")

	set(En, "form.back", "Back")
	set(Fr, "form.back", "Retour")
	set(Ar, "form.back", "رجوع")

	set(En, "form.name", "Automation name")
	set(Fr, "form.name", "Nom de l'automatisation")
	set(Ar, "form.name", "اسم الأتمتة")

	set(En, "form.namePlaceholder", "e.g. Delivery notification")
	set(Fr, "form.namePlaceholder", "ex. Notification de livraison")
	set(Ar, "form.namePlaceholder", "مثال: إشعار التوصيل")

	set(En, "form.description", "Description (optional)")
	set(Fr, "form.description", "Description (facultatif)")
	set(Ar, "form.description", "الوصف (اختياري)")

	set(En, "form.descriptionPlaceholder", "What this automation does")
	set(Fr, "form.descriptionPlaceholder", "Ce que fait cette automatisation")
	set(Ar, "form.descriptionPlaceholder", "ما تقوم به هذه الأتمتة")

	set(En, "form.sendType", "Send type")
	set(Fr, "form.sendType", "Type d'envoi")
	set(Ar, "form.sendType", "نوع الإرسال")

	set(En, "form.modeInstant", "Instant — send as soon as the event arrives")
	set(Fr, "form.modeInstant", "Immédiate — envoyée dès que l'événement arrive")
	set(Ar, "form.modeInstant", "فورية — تُرسل فور وصول الحدث")

	set(En, "form.modeFixed", "Scheduled — send at a fixed time on the chosen days")
	set(Fr, "form.modeFixed", "Programmée — envoyée à une heure fixe les jours choisis")
	set(Ar, "form.modeFixed", "مجدولة — تُرسل في وقت محدد في الأيام المختارة")

	set(En, "form.modeDelayed", "Delayed — send a few minutes after the event")
	set(Fr, "form.modeDelayed", "Différée — envoyée quelques minutes après l'événement")
	set(Ar, "form.modeDelayed", "مؤجلة — تُرسل بعد بضع دقائق من الحدث")

	set(En, "form.shop", "Shop")
	set(Fr, "form.shop", "Boutique")
	set(Ar, "form.shop", "المتجر")

	set(En, "form.eventSource", "Event source")
	set(Fr, "form.eventSource", "Source de l'événement")
	set(Ar, "form.eventSource", "مصدر الحدث")

	set(En, "form.sourceOrder", "Order event (Converty)")
	set(Fr, "form.sourceOrder", "Événement de commande (Converty)")
	set(Ar, "form.sourceOrder", "حدث طلب (Converty)")

	set(En, "form.sourceDelivery", "Delivery event (provider)")
	set(Fr, "form.sourceDelivery", "Événement de livraison (transporteur)")
	set(Ar, "form.sourceDelivery", "حدث توصيل (مزوّد)")

	set(En, "form.triggerOn", "Trigger on")
	set(Fr, "form.triggerOn", "Déclencher sur")
	set(Ar, "form.triggerOn", "التفعيل عند")

	set(En, "form.chooseStatus", "Choose a status…")
	set(Fr, "form.chooseStatus", "Choisissez un statut…")
	set(Ar, "form.chooseStatus", "اختر حالة…")

	set(En, "form.approvedTemplate", "Message (approved template)")
	set(Fr, "form.approvedTemplate", "Message (modèle approuvé)")
	set(Ar, "form.approvedTemplate", "الرسالة (قالب معتمد)")

	set(En, "form.noTemplateOther", "No approved template for this event source. Switch the event source, or create one in the Templates menu.")
	set(Fr, "form.noTemplateOther", "Aucun modèle approuvé pour cette source d'événement. Changez de source, ou créez-en un dans le menu Modèles.")
	set(Ar, "form.noTemplateOther", "لا يوجد قالب معتمد لمصدر الحدث هذا. بدّل مصدر الحدث أو أنشئ قالبًا من قائمة القوالب.")

	set(En, "form.noTemplateAny", "Create and get a template approved in the Templates menu first — an automation can only send an approved template.")
	set(Fr, "form.noTemplateAny", "Créez et faites approuver un modèle dans le menu Modèles d'abord — une automatisation ne peut envoyer qu'un modèle approuvé.")
	set(Ar, "form.noTemplateAny", "أنشئ قالبًا واعتمده من قائمة القوالب أولاً — الأتمتة لا تُرسل إلا قالبًا معتمدًا.")

	set(En, "form.noTemplateShop", "This shop has no approved template. Create and get one approved in the Templates menu.")
	set(Fr, "form.noTemplateShop", "Cette boutique n'a aucun modèle approuvé. Créez-en un et faites-le approuver dans le menu Modèles.")
	set(Ar, "form.noTemplateShop", "لا يملك هذا المتجر قالبًا معتمدًا. أنشئ قالبًا واعتمده من قائمة القوالب.")

	set(En, "form.chooseTemplate", "Choose a template…")
	set(Fr, "form.chooseTemplate", "Choisissez un modèle…")
	set(Ar, "form.chooseTemplate", "اختر قالبًا…")

	set(En, "form.scheduleSettings", "Schedule settings")
	set(Fr, "form.scheduleSettings", "Paramètres de planification")
	set(Ar, "form.scheduleSettings", "إعدادات الجدولة")

	set(En, "form.sendDays", "Send days")
	set(Fr, "form.sendDays", "Jours d'envoi")
	set(Ar, "form.sendDays", "أيام الإرسال")

	set(En, "form.sendTime", "Send time")
	set(Fr, "form.sendTime", "Heure d'envoi")
	set(Ar, "form.sendTime", "وقت الإرسال")

	set(En, "form.timezone", "Timezone")
	set(Fr, "form.timezone", "Fuseau horaire")
	set(Ar, "form.timezone", "المنطقة الزمنية")

	set(En, "form.fixedHint", "An event arriving before the time waits until it; an event at or after it is sent immediately. On a day you didn't select, the send waits for the next selected day at the same time.")
	set(Fr, "form.fixedHint", "Un événement arrivant avant l'heure attend jusqu'à celle-ci ; un événement à cette heure ou après est envoyé immédiatement. Un jour non sélectionné, l'envoi attend le prochain jour choisi à la même heure.")
	set(Ar, "form.fixedHint", "الحدث الذي يصل قبل الوقت ينتظر حتى حلوله؛ والحدث الذي يصل في الوقت نفسه أو بعده يُرسل فورًا. في يوم لم تحدده، ينتظر الإرسال اليومَ المحدد التالي في نفس الوقت.")

	set(En, "form.delayAfter", "Delay after the event (minutes)")
	set(Fr, "form.delayAfter", "Délai après l'événement (minutes)")
	set(Ar, "form.delayAfter", "التأخير بعد الحدث (بالدقائق)")

	set(En, "form.delayHint", "The message is sent %s minutes after the event, even if the daily send time has passed.")
	set(Fr, "form.delayHint", "Le message est envoyé %s minutes après l'événement, même si l'heure d'envoi quotidienne est passée.")
	set(Ar, "form.delayHint", "تُرسل الرسالة بعد %s دقيقة من الحدث، حتى لو مضى وقت الإرسال اليومي.")

	set(En, "form.save", "Save")
	set(Fr, "form.save", "Enregistrer")
	set(Ar, "form.save", "حفظ")

	set(En, "form.create", "Create automation")
	set(Fr, "form.create", "Créer l'automatisation")
	set(Ar, "form.create", "إنشاء الأتمتة")

	set(En, "form.previewTitle", "Message preview")
	set(Fr, "form.previewTitle", "Aperçu du message")
	set(Ar, "form.previewTitle", "معاينة الرسالة")

	set(En, "form.previewCaption", "What the customer will receive")
	set(Fr, "form.previewCaption", "Ce que recevra le client")
	set(Ar, "form.previewCaption", "ما سيستلمه العميل")

	set(En, "form.previewEmpty", "Select a template to preview.")
	set(Fr, "form.previewEmpty", "Sélectionnez un modèle pour l'aperçu.")
	set(Ar, "form.previewEmpty", "اختر قالبًا للمعاينة.")

	set(En, "form.previewHint", "The variables you drop into the message when writing a template are filled automatically from the order or parcel that triggered the send.")
	set(Fr, "form.previewHint", "Les variables insérées dans le message lors de la rédaction du modèle sont remplies automatiquement à partir de la commande ou du colis qui a déclenché l'envoi.")
	set(Ar, "form.previewHint", "تُملأ المتغيرات التي تُدرجها في الرسالة عند كتابة القالب تلقائيًا من الطلب أو الطرد الذي شغَّل الإرسال.")

	// --- client-side form validation (injected into the form script) ---
	set(En, "form.clockNow", "Right now it is %s in %s — a send at %s leaves at %s there.")
	set(Fr, "form.clockNow", "Il est actuellement %s à %s — un envoi à %s part à %s (heure locale).")
	set(Ar, "form.clockNow", "الآن الساعة %s في %s — إرسال عند %s يُرسل عند %s هناك.")

	set(En, "form.clockUnreadable", "Could not read the clock for %s.")
	set(Fr, "form.clockUnreadable", "Impossible de lire l'horloge pour %s.")
	set(Ar, "form.clockUnreadable", "تعذّرت قراءة الساعة لمنطقة %s.")

	set(En, "form.noTemplate", "No approved template for this shop yet.")
	set(Fr, "form.noTemplate", "Aucun modèle approuvé pour cette boutique pour le moment.")
	set(Ar, "form.noTemplate", "لا يوجد قالب معتمد لهذا المتجر بعد.")

	set(En, "form.needTime", "A scheduled send needs a send time.")
	set(Fr, "form.needTime", "Un envoi programmé nécessite une heure d'envoi.")
	set(Ar, "form.needTime", "الإرسال المجدول يحتاج إلى وقت إرسال.")

	set(En, "form.needDay", "A scheduled send needs at least one send day.")
	set(Fr, "form.needDay", "Un envoi programmé nécessite au moins un jour d'envoi.")
	set(Ar, "form.needDay", "الإرسال المجدول يحتاج إلى يوم إرسال واحد على الأقل.")

	set(En, "form.needTz", "Pick the timezone the send time is in.")
	set(Fr, "form.needTz", "Choisissez le fuseau horaire de l'heure d'envoi.")
	set(Ar, "form.needTz", "اختر المنطقة الزمنية لوقت الإرسال.")

	set(En, "form.needDelay", "A delayed send needs a delay of at least one minute.")
	set(Fr, "form.needDelay", "Un envoi différé nécessite un délai d'au moins une minute.")
	set(Ar, "form.needDelay", "الإرسال المؤجل يحتاج إلى تأخير دقيقة واحدة على الأقل.")

	// --- send history ---
	set(En, "hist.statusDelivered", "Delivered")
	set(Fr, "hist.statusDelivered", "Livré")
	set(Ar, "hist.statusDelivered", "تم التسليم")

	set(En, "hist.statusRead", "Read")
	set(Fr, "hist.statusRead", "Lu")
	set(Ar, "hist.statusRead", "مقروءة")

	set(En, "hist.statusSent", "Sent")
	set(Fr, "hist.statusSent", "Envoyé")
	set(Ar, "hist.statusSent", "أُرسلت")

	set(En, "hist.statusFailed", "Failed")
	set(Fr, "hist.statusFailed", "Échec")
	set(Ar, "hist.statusFailed", "فشل")

	set(En, "hist.statusNotSent", "Not sent")
	set(Fr, "hist.statusNotSent", "Non envoyé")
	set(Ar, "hist.statusNotSent", "لم تُرسل")

	set(En, "hist.allAutomations", "← All automations")
	set(Fr, "hist.allAutomations", "← Toutes les automatisations")
	set(Ar, "hist.allAutomations", "→ كل الأتمتة")

	set(En, "hist.sendHistory", "send history")
	set(Fr, "hist.sendHistory", "historique des envois")
	set(Ar, "hist.sendHistory", "سجل الإرسال")

	set(En, "hist.subtitle", "Every message this automation has sent, newest first.")
	set(Fr, "hist.subtitle", "Chaque message envoyé par cette automatisation, du plus récent au plus ancien.")
	set(Ar, "hist.subtitle", "كل رسالة أرسلتها هذه الأتمتة، الأحدث أولاً.")

	set(En, "hist.trigger", "Trigger:")
	set(Fr, "hist.trigger", "Déclencheur :")
	set(Ar, "hist.trigger", "المشغّل:")

	set(En, "hist.template", "template")
	set(Fr, "hist.template", "modèle")
	set(Ar, "hist.template", "القالب")

	set(En, "hist.editAutomation", "Edit automation")
	set(Fr, "hist.editAutomation", "Modifier l'automatisation")
	set(Ar, "hist.editAutomation", "تعديل الأتمتة")

	set(En, "hist.total", "Total")
	set(Fr, "hist.total", "Total")
	set(Ar, "hist.total", "الإجمالي")

	set(En, "hist.sent", "Sent")
	set(Fr, "hist.sent", "Envoyés")
	set(Ar, "hist.sent", "أُرسل")

	set(En, "hist.delivered", "Delivered")
	set(Fr, "hist.delivered", "Livrés")
	set(Ar, "hist.delivered", "تم التسليم")

	set(En, "hist.read", "Read")
	set(Fr, "hist.read", "Lus")
	set(Ar, "hist.read", "مقروء")

	set(En, "hist.failed", "Failed")
	set(Fr, "hist.failed", "Échecs")
	set(Ar, "hist.failed", "فشل")

	set(En, "hist.notSent", "Not sent")
	set(Fr, "hist.notSent", "Non envoyés")
	set(Ar, "hist.notSent", "لم تُرسل")

	set(En, "hist.heldBack", "Held back")
	set(Fr, "hist.heldBack", "Retenus")
	set(Ar, "hist.heldBack", "محتجَزة")

	set(En, "hist.ruleInstant", "sends as soon as the event happens")
	set(Fr, "hist.ruleInstant", "envoie dès que l'événement se produit")
	set(Ar, "hist.ruleInstant", "تُرسل فور حدوث الحدث")

	set(En, "hist.ruleDelay", "sends %d minutes after the event")
	set(Fr, "hist.ruleDelay", "envoie %d minutes après l'événement")
	set(Ar, "hist.ruleDelay", "تُرسل بعد %d دقيقة من الحدث")

	set(En, "hist.ruleFixed", "sends at %s local")
	set(Fr, "hist.ruleFixed", "envoie à %s (heure locale)")
	set(Ar, "hist.ruleFixed", "تُرسل عند %s محليًا")

	set(En, "hist.note", "This automation %s. Messages appear here only once they have been sent, so an empty list before the first one is normal.")
	set(Fr, "hist.note", "Cette automatisation %s. Les messages n'apparaissent ici qu'une fois envoyés : une liste vide avant le premier envoi est donc normale.")
	set(Ar, "hist.note", "هذه الأتمتة %s. تظهر الرسائل هنا فقط بعد إرسالها، لذا من الطبيعي أن تكون القائمة فارغة قبل الأولى.")

	set(En, "hist.sentNow", "%d message(s) sent now.")
	set(Fr, "hist.sentNow", "%d message(s) envoyé(s) maintenant.")
	set(Ar, "hist.sentNow", "%d رسالة أُرسلت الآن.")

	set(En, "hist.onWay", "%d message(s) are on their way now.")
	set(Fr, "hist.onWay", "%d message(s) sont en route maintenant.")
	set(Ar, "hist.onWay", "%d رسالة في الطريق الآن.")

	set(En, "hist.cannotSend", "This automation cannot send at all right now.")
	set(Fr, "hist.cannotSend", "Cette automatisation ne peut pas envoyer du tout pour le moment.")
	set(Ar, "hist.cannotSend", "لا يمكن لهذه الأتمتة الإرسال الآن إطلاقًا.")

	set(En, "hist.nothingNew", "Nothing new could be sent.")
	set(Fr, "hist.nothingNew", "Rien de nouveau n'a pu être envoyé.")
	set(Ar, "hist.nothingNew", "تعذّر إرسال أي شيء جديد.")

	set(En, "hist.needTemplate", "%d need the template this automation points at. Choose another template on the automation — retrying will not help.")
	set(Fr, "hist.needTemplate", "%d ont besoin du modèle visé par cette automatisation. Choisissez un autre modèle sur l'automatisation — réessayer n'aidera pas.")
	set(Ar, "hist.needTemplate", "%d تحتاج إلى القالب الذي تشير إليه هذه الأتمتة. اختر قالبًا آخر على الأتمتة — إعادة المحاولة لن تنفع.")

	set(En, "hist.stillHeld", "%d still held back — the template needs data this order does not have yet.")
	set(Fr, "hist.stillHeld", "%d toujours retenus — le modèle attend des données que cette commande n'a pas encore.")
	set(Ar, "hist.stillHeld", "%d ما زالت محتجزة — القالب يحتاج إلى بيانات لا يملكها هذا الطلب بعد.")

	set(En, "hist.rejected", "%d were refused by the send rules (consent, or a template the carrier treats as marketing). Retrying will not change it.")
	set(Fr, "hist.rejected", "%d ont été refusées par les règles d'envoi (consentement, ou modèle que le transporteur traite comme du marketing). Réessayer ne changera rien.")
	set(Ar, "hist.rejected", "%d رفضتها قواعد الإرسال (الموافقة، أو قالب يعتبره المزوّد تسويقيًا). إعادة المحاولة لن تغيّر شيئًا.")

	set(En, "hist.alreadySent", "%d had already been sent and were left alone.")
	set(Fr, "hist.alreadySent", "%d avaient déjà été envoyées et ont été laissées telles quelles.")
	set(Ar, "hist.alreadySent", "%d سبق إرسالها وتُركت كما هي.")

	set(En, "hist.nothingYet", "Nothing sent yet")
	set(Fr, "hist.nothingYet", "Rien d'envoyé pour le moment")
	set(Ar, "hist.nothingYet", "لم يُرسل شيء بعد")

	set(En, "hist.heldEvents", "%d events matched this automation but were held back rather than sent. They are listed below with what was missing.")
	set(Fr, "hist.heldEvents", "%d événements correspondaient à cette automatisation mais ont été retenus au lieu d'être envoyés. Ils sont listés ci-dessous avec ce qui manquait.")
	set(Ar, "hist.heldEvents", "%d أحداث طابقت هذه الأتمتة لكنها رُفضت إرسالاً. وهي مدرجة أدناه مع ما نقص.")

	set(En, "hist.noEvent", "No event has triggered a message from this automation yet. A send is recorded only when it actually goes out, so an empty list here means \"no matching event\" — not a broken automation.")
	set(Fr, "hist.noEvent", "Aucun événement n'a encore déclenché de message de cette automatisation. Un envoi n'est enregistré que lorsqu'il part réellement : une liste vide signifie donc \"aucun événement correspondant\" — pas une automatisation en panne.")
	set(Ar, "hist.noEvent", "لم يُشغِّل أي حدث رسالة من هذه الأتمتة بعد. يُسجَّل الإرسال فقط عندما يُرسل فعلًا، لذا القائمة الفارغة هنا تعني «لا يوجد حدث مطابق» — وليس عطلًا في الأتمتة.")

	set(En, "hist.when", "When")
	set(Fr, "hist.when", "Quand")
	set(Ar, "hist.when", "متى")

	set(En, "hist.customer", "Customer")
	set(Fr, "hist.customer", "Client")
	set(Ar, "hist.customer", "العميل")

	set(En, "hist.phone", "Phone")
	set(Fr, "hist.phone", "Téléphone")
	set(Ar, "hist.phone", "الهاتف")

	set(En, "hist.order", "Order")
	set(Fr, "hist.order", "Commande")
	set(Ar, "hist.order", "الطلب")

	set(En, "hist.status", "Status")
	set(Fr, "hist.status", "Statut")
	set(Ar, "hist.status", "الحالة")

	set(En, "hist.deliveredAt", "Delivered")
	set(Fr, "hist.deliveredAt", "Livré")
	set(Ar, "hist.deliveredAt", "تم التسليم")

	set(En, "hist.message", "Message")
	set(Fr, "hist.message", "Message")
	set(Ar, "hist.message", "الرسالة")

	set(En, "hist.recordedNotConfirmed", "Recorded, but WhatsApp never confirmed it — this send did not complete.")
	set(Fr, "hist.recordedNotConfirmed", "Enregistré, mais WhatsApp ne l'a jamais confirmé — cet envoi n'a pas abouti.")
	set(Ar, "hist.recordedNotConfirmed", "مُسجَّل، لكن واتساب لم يؤكده أبدًا — لم يكتمل هذا الإرسال.")

	set(En, "hist.readByCustomer", "Read by the customer")
	set(Fr, "hist.readByCustomer", "Lu par le client")
	set(Ar, "hist.readByCustomer", "قرأها العميل")

	set(En, "hist.acceptedByWhatsapp", "Accepted by WhatsApp")
	set(Fr, "hist.acceptedByWhatsapp", "Accepté par WhatsApp")
	set(Ar, "hist.acceptedByWhatsapp", "قبلها واتساب")

	set(En, "hist.showingRecent", "Showing the 200 most recent messages.")
	set(Fr, "hist.showingRecent", "Affichage des 200 messages les plus récents.")
	set(Ar, "hist.showingRecent", "عرض أحدث 200 رسالة.")

	set(En, "hist.heldTitle", "Held back — matched, but not sent")
	set(Fr, "hist.heldTitle", "Retenus — correspondants, mais non envoyés")
	set(Ar, "hist.heldTitle", "محتجَزة — طابقت لكن لم تُرسل")

	set(En, "hist.heldBody", "These events reached this automation, but the message was withheld on purpose: the template places a variable that this order had no value for, and a blank line in the customer's message is worse than no message. Nothing was sent to anyone on this list.")
	set(Fr, "hist.heldBody", "Ces événements ont atteint cette automatisation, mais l'envoi a été volontairement retenu : le modèle place une variable que cette commande n'a pas de valeur pour, et une ligne vide dans le message du client est pire que pas de message. Personne sur cette liste n'a rien reçu.")
	set(Ar, "hist.heldBody", "وصلت هذه الأحداث إلى هذه الأتمتة، لكن الرسالة حُجِزت عمدًا: القالب يضع متغيرًا لا يملك هذا الطلب قيمة له، وسطر فارغ في رسالة العميل أسوأ من عدم وجود رسالة. لم يُرسَل أي شيء إلى أي شخص في هذه القائمة.")

	set(En, "hist.sendHeldNow", "Send held back now")
	set(Fr, "hist.sendHeldNow", "Envoyer maintenant les retenus")
	set(Ar, "hist.sendHeldNow", "إرسال المحجوز الآن")

	set(En, "hist.heldWaitingFor", "Waiting for")
	set(Fr, "hist.heldWaitingFor", "En attente de")
	set(Ar, "hist.heldWaitingFor", "بانتظار")

	set(En, "hist.why", "Why")
	set(Fr, "hist.why", "Pourquoi")
	set(Ar, "hist.why", "السبب")

	set(En, "hist.heldFootnote", "A held-back send is not an error, and it waits for nothing: no new event will come for an order that has already changed status. Once the missing detail is available — a driver attached by the carrier, for example — use the button above and it goes out immediately.")
	set(Fr, "hist.heldFootnote", "Un envoi retenu n'est pas une erreur et n'attend rien : aucun nouvel événement n'arrivera pour une commande qui a déjà changé de statut. Dès que le détail manquant est disponible — un chauffeur ajouté par le transporteur, par exemple — utilisez le bouton ci-dessus et le message part immédiatement.")
	set(Ar, "hist.heldFootnote", "الإرسال المحجوز ليس خطأً ولا ينتظر شيئًا: لن يصل حدث جديد لطلبٍ غيّر حالته بالفعل. بمجرد توفر التفاصيل الناقصة — سائق مرفق من قبل المزوّد مثلاً — استخدم الزر أعلاه وستُرسل الرسالة فورًا.")

	// --- generic ---
	set(En, "common.cancel", "Cancel")
	set(Fr, "common.cancel", "Annuler")
	set(Ar, "common.cancel", "إلغاء")

	set(En, "common.comingSoonBody", "This module lands in a later phase. It will let you manage %s from your dashboard.")
	set(Fr, "common.comingSoonBody", "Ce module arrive dans une phase ultérieure. Il vous permettra de gérer %s depuis votre tableau de bord.")
	set(Ar, "common.comingSoonBody", "هذه الوحدة ستُضاف في مرحلة لاحقة. ستتيح لك إدارة %s من لوحة التحكم.")
}