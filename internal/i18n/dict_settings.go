package i18n

// Settings pages: WhatsApp connection (was.*), delivery provider & watched
// parcels (dlv.*), and the WhatsApp onboarding/consent flow (onb.*).
func init() {
	set(En, "was.title", "WhatsApp settings")
	set(Fr, "was.title", "Paramètres WhatsApp")
	set(Ar, "was.title", "إعدادات واتساب")

	set(En, "was.subtitle", "Connect your WhatsApp Business number once — it is shared by every connected store.")
	set(Fr, "was.subtitle", "Connectez votre numéro WhatsApp Business une seule fois : il est partagé par toutes les boutiques connectées.")
	set(Ar, "was.subtitle", "اربط رقم واتساب للأعمال مرة واحدة؛ فهو مشترك بين جميع المتاجر المتصلة.")

	set(En, "was.connect", "Connect WhatsApp")
	set(Fr, "was.connect", "Connecter WhatsApp")
	set(Ar, "was.connect", "ربط واتساب")

	set(En, "was.connectBody", "Paste the credentials of the WhatsApp Business number your stores send through. This one number and its approved templates are used by every connected store.")
	set(Fr, "was.connectBody", "Collez les identifiants du numéro WhatsApp Business par lequel vos boutiques envoient leurs messages. Ce numéro et ses modèles approuvés sont utilisés par chaque boutique connectée.")
	set(Ar, "was.connectBody", "الصق بيانات الاعتماد لرقم الواتساب للأعمال الذي ترسل من خلاله متاجرك. يُستخدم هذا الرقم وقوالبه المعتمدة في كل متجر متصل.")

	set(En, "was.step1", "Meta Business Manager or developer app: create a long-lived token (a temporary token from the app dashboard expires within hours).")
	set(Fr, "was.step1", "Meta Business Manager ou application développeur : créez un jeton de longue durée (un jeton temporaire du tableau de bord expire en quelques heures).")
	set(Ar, "was.step1", "Meta Business Manager أو تطبيق المطور: أنشئ رمزًا طويل الأجل (الرمز المؤقت من لوحة التطبيق ينتهي خلال ساعات).")

	set(En, "was.step2", "Copy the Phone number ID and the WABA (WhatsApp Business Account) ID of that same number.")
	set(Fr, "was.step2", "Copiez l'identifiant du numéro et l'identifiant WABA (compte WhatsApp Business) de ce même numéro.")
	set(Ar, "was.step2", "انسخ معرّف رقم الهاتف ومعرّف WABA (حساب واتساب للأعمال) لنفس الرقم.")

	set(En, "was.step3", "Paste all three below and save.")
	set(Fr, "was.step3", "Collez les trois ci-dessous et enregistrez.")
	set(Ar, "was.step3", "الصق الثلاثة أدناه ثم احفظ.")

	set(En, "was.token", "Meta access token")
	set(Fr, "was.token", "Jeton d'accès Meta")
	set(Ar, "was.token", "رمز الوصول من Meta")

	set(En, "was.phoneNumberId", "Phone number ID")
	set(Fr, "was.phoneNumberId", "Identifiant du numéro")
	set(Ar, "was.phoneNumberId", "معرّف رقم الهاتف")

	set(En, "was.wabaId", "WABA (messaging account) ID")
	set(Fr, "was.wabaId", "Identifiant WABA (compte de messagerie)")
	set(Ar, "was.wabaId", "معرّف WABA (حساب المراسلة)")

	set(En, "was.wabaPlaceholder", "your WhatsApp Business Account ID")
	set(Fr, "was.wabaPlaceholder", "l'identifiant de votre compte WhatsApp Business")
	set(Ar, "was.wabaPlaceholder", "معرّف حساب واتساب للأعمال")

	set(En, "was.connected", "Connected")
	set(Fr, "was.connected", "Connecté")
	set(Ar, "was.connected", "متصل")

	set(En, "was.disconnectConfirm", "Disconnect your WhatsApp number? Order confirmation messages will stop sending.")
	set(Fr, "was.disconnectConfirm", "Déconnecter votre numéro WhatsApp ? Les messages de confirmation de commande ne seront plus envoyés.")
	set(Ar, "was.disconnectConfirm", "هل تريد فصل رقم واتساب؟ ستتوقف رسائل تأكيد الطلبات عن الإرسال.")

	set(En, "was.disconnect", "Disconnect")
	set(Fr, "was.disconnect", "Déconnecter")
	set(Ar, "was.disconnect", "فصل")

	set(En, "was.number", "WhatsApp number")
	set(Fr, "was.number", "Numéro WhatsApp")
	set(Ar, "was.number", "رقم واتساب")

	set(En, "was.status", "Status")
	set(Fr, "was.status", "Statut")
	set(Ar, "was.status", "الحالة")

	set(En, "was.available", "Available templates (%d)")
	set(Fr, "was.available", "Modèles disponibles (%d)")
	set(Ar, "was.available", "القوالب المتاحة (%d)")

	set(En, "was.recent", "Recent webhook deliveries")
	set(Fr, "was.recent", "Livraisons webhook récentes")
	set(Ar, "was.recent", "عمليات تسليم الويبهوك الحديثة")

	set(En, "was.recentBody", "Every Meta webhook POST is recorded here before processing, so a delivery that is rejected or fails to map to a number is visible instead of silently vanishing.")
	set(Fr, "was.recentBody", "Chaque POST webhook Meta est enregistré ici avant traitement : une livraison rejetée ou sans numéro est donc visible au lieu de disparaître silencieusement.")
	set(Ar, "was.recentBody", "يُسجَّل كل طلب ويبهوك من Meta هنا قبل المعالجة، فتظهر أي رسالة مرفوضة أو بدون رقم بدل أن تختفي بصمت.")

	set(En, "was.none", "No webhook deliveries yet.")
	set(Fr, "was.none", "Aucune livraison webhook pour l'instant.")
	set(Ar, "was.none", "لا توجد عمليات ويبهوك بعد.")

	set(En, "onb.title", "Enable WhatsApp messaging")
	set(Fr, "onb.title", "Activer la messagerie WhatsApp")
	set(Ar, "onb.title", "تفعيل مراسلة واتساب")

	set(En, "onb.subtitleA", "Your messages are sent through ")
	set(Fr, "onb.subtitleA", "Vos messages sont envoyés via ")
	set(Ar, "onb.subtitleA", "تُرسل رسائلك عبر ")

	set(En, "onb.subtitleOur", "our")
	set(Fr, "onb.subtitleOur", "notre")
	set(Ar, "onb.subtitleOur", "رقم منصتنا")

	set(En, "onb.subtitleB", " WhatsApp platform number. You do not need to connect or manage a Meta app.")
	set(Fr, "onb.subtitleB", " numéro WhatsApp de la plateforme. Vous n'avez pas à connecter ni gérer d'application Meta.")
	set(Ar, "onb.subtitleB", " على واتساب. لا تحتاج إلى ربط أو إدارة تطبيق Meta.")

	set(En, "onb.enabled", "WhatsApp service is enabled for this shop.")
	set(Fr, "onb.enabled", "Le service WhatsApp est activé pour cette boutique.")
	set(Ar, "onb.enabled", "خدمة واتساب مفعّلة لهذا المتجر.")

	set(En, "onb.phoneLabel", "Shop contact phone (shown in messages)")
	set(Fr, "onb.phoneLabel", "Téléphone de contact de la boutique (affiché dans les messages)")
	set(Ar, "onb.phoneLabel", "هاتف تواصل المتجر (يظهر في الرسائل)")

	set(En, "onb.consentA", "I confirm that the phone numbers I message were collected with the customers' ")
	set(Fr, "onb.consentA", "Je confirme que les numéros que je contacte ont été collectés avec le ")
	set(Ar, "onb.consentA", "أؤكد أن أرقام الهواتف التي أرسل إليها جُمعت بموافقة العملاء (")

	set(En, "onb.consentB", "WhatsApp opt-in consent")
	set(Fr, "onb.consentB", "consentement opt-in WhatsApp")
	set(Ar, "onb.consentB", "الموافقة الصريحة على واتساب")

	set(En, "onb.consentC", ", that I will only send approved transactional templates, and that I accept the platform's ")
	set(Fr, "onb.consentC", "), que j'enverrai uniquement des modèles transactionnels approuvés et que j'accepte les ")
	set(Ar, "onb.consentC", ")، وأنني سأرسل فقط قوالب معاملات معتمدة، وأنني أوافق على ")

	set(En, "onb.consentTerms", "terms")
	set(Fr, "onb.consentTerms", "conditions")
	set(Ar, "onb.consentTerms", "شروط")

	set(En, "onb.submit", "Enable WhatsApp messaging")
	set(Fr, "onb.submit", "Activer la messagerie WhatsApp")
	set(Ar, "onb.submit", "تفعيل مراسلة واتساب")

	set(En, "dlv.title", "Delivery")
	set(Fr, "dlv.title", "Livraison")
	set(Ar, "dlv.title", "التوصيل")

	set(En, "dlv.subtitle", "Connect a delivery provider so parcel status changes fire your order-event automations. First provider: Mes Colis Express.")
	set(Fr, "dlv.subtitle", "Connectez un prestataire de livraison pour que les changements de statut déclenchent vos automatisations. Premier prestataire : Mes Colis Express.")
	set(Ar, "dlv.subtitle", "اربط مزوّد توصيل بحيث تُطلق تغييرات حالة الطرد أتمتة أحداث الطلبات. أول مزوّد: Mes Colis Express.")

	set(En, "dlv.connect", "Connect Mes Colis Express")
	set(Fr, "dlv.connect", "Connecter Mes Colis Express")
	set(Ar, "dlv.connect", "ربط Mes Colis Express")

	set(En, "dlv.connectBody", "Paste the API access token of your Mes Colis account. The token is stored encrypted and is validated against the Mes Colis API before saving. Parcels are polled for status changes every couple of minutes.")
	set(Fr, "dlv.connectBody", "Collez le jeton API de votre compte Mes Colis. Il est stocké chiffré et validé auprès de l'API Mes Colis avant enregistrement. Les colis sont interrogés toutes les deux minutes.")
	set(Ar, "dlv.connectBody", "الصق رمز الوصول إلى API الخاص بحسابك في Mes Colis. يُخزَّن الرمز مشفّرًا ويتم التحقق منه عبر API قبل الحفظ. تُفحص الطرود كل بضع دقائق بحثًا عن تغييرات الحالة.")

	set(En, "dlv.tokenLabel", "Mes Colis access token (x-access-token)")
	set(Fr, "dlv.tokenLabel", "Jeton d'accès Mes Colis (x-access-token)")
	set(Ar, "dlv.tokenLabel", "رمز الوصول لـ Mes Colis (x-access-token)")

	set(En, "dlv.tokenPlaceholder", "Paste your Mes Colis access token")
	set(Fr, "dlv.tokenPlaceholder", "Collez votre jeton d'accès Mes Colis")
	set(Ar, "dlv.tokenPlaceholder", "الصق رمز الوصول الخاص بـ Mes Colis")

	set(En, "dlv.accountCode", "Account code (optional)")
	set(Fr, "dlv.accountCode", "Code de compte (optionnel)")
	set(Ar, "dlv.accountCode", "رمز الحساب (اختياري)")

	set(En, "dlv.accountCodePlaceholder", "For sub-account access")
	set(Fr, "dlv.accountCodePlaceholder", "Pour l'accès sous-compte")
	set(Ar, "dlv.accountCodePlaceholder", "للوصول إلى الحساب الفرعي")

	set(En, "dlv.subAccount", "Use sub-account access")
	set(Fr, "dlv.subAccount", "Utiliser l'accès sous-compte")
	set(Ar, "dlv.subAccount", "استخدام وصول الحساب الفرعي")

	set(En, "dlv.connected", "Mes Colis connected")
	set(Fr, "dlv.connected", "Mes Colis connecté")
	set(Ar, "dlv.connected", "تم ربط Mes Colis")

	set(En, "dlv.disconnectConfirm", "Disconnect Mes Colis? Delivery automations will stop firing.")
	set(Fr, "dlv.disconnectConfirm", "Déconnecter Mes Colis ? Les automatisations de livraison cesseront de se déclencher.")
	set(Ar, "dlv.disconnectConfirm", "هل تريد فصل Mes Colis؟ ستتوقف أتمتة التوصيل عن العمل.")

	set(En, "dlv.connectedBody", "Parcel statuses are polled automatically. A parcel starts being tracked by itself on every Converty upload event that carries the tracking reference — the barcode, order id, customer name and phone are all captured from the order, so there is nothing to enter here.")
	set(Fr, "dlv.connectedBody", "Les statuts des colis sont interrogés automatiquement. Un colis est suivi dès qu'un événement Converty porte sa référence de suivi : code-barres, commande, client et téléphone sont capturés depuis la commande, rien à saisir ici.")
	set(Ar, "dlv.connectedBody", "تُفحص حالات الطرود تلقائيًا. يبدأ تتبّع الطرد تلقائيًا مع كل حدث رفع من Converty يحمل الرقم المرجعي للتتبع؛ تُلتقط الرموز الشريطية ومعرّف الطلب واسم العميل وهاتفه من الطلب، فلا حاجة لإدخال شيء هنا.")

	set(En, "dlv.tracked", "Tracked parcels")
	set(Fr, "dlv.tracked", "Colis suivis")
	set(Ar, "dlv.tracked", "الطرود المتتبَّعة")

	set(En, "dlv.trackedHint", "Parcels appear when a Converty order event carries a tracking reference. You can remove one here to stop polling it.")
	set(Fr, "dlv.trackedHint", "Les colis apparaissent quand un événement de commande Converty porte une référence de suivi. Vous pouvez en retirer un ici pour cesser de l'interroger.")
	set(Ar, "dlv.trackedHint", "تظهر الطرود عندما يحمل حدث طلب من Converty رقمًا مرجعيًا للتتبع. يمكنك إزالة أي طرد هنا لإيقاف فحصه.")

	set(En, "dlv.checkNow", "Check now")
	set(Fr, "dlv.checkNow", "Vérifier maintenant")
	set(Ar, "dlv.checkNow", "فحص الآن")

	set(En, "dlv.empty", "No parcels being watched yet. Once a Converty order event carries a tracking reference, the parcel appears here automatically.")
	set(Fr, "dlv.empty", "Aucun colis suivi pour l'instant. Dès qu'un événement de commande Converty porte une référence de suivi, le colis apparaît ici automatiquement.")
	set(Ar, "dlv.empty", "لا توجد طرود قيد التتبع بعد. بمجرد أن يحمل حدث طلب من Converty رقمًا مرجعيًا للتتبع، يظهر الطرد هنا تلقائيًا.")

	set(En, "dlv.colShop", "Shop")
	set(Fr, "dlv.colShop", "Boutique")
	set(Ar, "dlv.colShop", "المتجر")

	set(En, "dlv.colBarcode", "Barcode")
	set(Fr, "dlv.colBarcode", "Code-barres")
	set(Ar, "dlv.colBarcode", "الباركود")

	set(En, "dlv.colOrder", "Order")
	set(Fr, "dlv.colOrder", "Commande")
	set(Ar, "dlv.colOrder", "الطلب")

	set(En, "dlv.colCustomer", "Customer")
	set(Fr, "dlv.colCustomer", "Client")
	set(Ar, "dlv.colCustomer", "العميل")

	set(En, "dlv.colStatus", "Status")
	set(Fr, "dlv.colStatus", "Statut")
	set(Ar, "dlv.colStatus", "الحالة")

	set(En, "dlv.colDriver", "Driver")
	set(Fr, "dlv.colDriver", "Livreur")
	set(Ar, "dlv.colDriver", "السائق")

	set(En, "dlv.colSeen", "Last seen")
	set(Fr, "dlv.colSeen", "Dernier contact")
	set(Ar, "dlv.colSeen", "آخر ظهور")

	set(En, "dlv.firstPoll", "waiting for first poll")
	set(Fr, "dlv.firstPoll", "en attente du premier relevé")
	set(Ar, "dlv.firstPoll", "في انتظار أول فحص")

	set(En, "dlv.noName", "name not reported")
	set(Fr, "dlv.noName", "nom non communiqué")
	set(Ar, "dlv.noName", "الاسم غير مُبلَّغ")

	set(En, "dlv.unassigned", "not assigned")
	set(Fr, "dlv.unassigned", "non attribué")
	set(Ar, "dlv.unassigned", "غير معيّن")

	set(En, "dlv.missing", "not listed at the carrier,")
	set(Fr, "dlv.missing", "absent chez le transporteur,")
	set(Ar, "dlv.missing", "غير مدرج لدى شركة التوصيل،")

	set(En, "dlv.removedUpstream", "deleted at the carrier, no longer polled")
	set(Fr, "dlv.removedUpstream", "supprimé chez le transporteur, plus interrogé")
	set(Ar, "dlv.removedUpstream", "محذوف لدى شركة التوصيل، لم يعد يُفحص")

	set(En, "dlv.terminalNote", "final, no longer polled")
	set(Fr, "dlv.terminalNote", "final, plus interrogé")
	set(Ar, "dlv.terminalNote", "نهائي، لم يعد يُفحص")

	set(En, "dlv.removeConfirm", "Stop tracking this parcel?")
	set(Fr, "dlv.removeConfirm", "Arrêter le suivi de ce colis ?")
	set(Ar, "dlv.removeConfirm", "هل تريد إيقاف تتبع هذا الطرد؟")

	set(En, "dlv.remove", "Remove")
	set(Fr, "dlv.remove", "Retirer")
	set(Ar, "dlv.remove", "إزالة")

	set(En, "dlv.justNow", "just now")
	set(Fr, "dlv.justNow", "à l'instant")
	set(Ar, "dlv.justNow", "الآن")

	set(En, "dlv.minutesAgo", "%dm ago")
	set(Fr, "dlv.minutesAgo", "il y a %dm")
	set(Ar, "dlv.minutesAgo", "منذ %d د")

	set(En, "dlv.hoursAgo", "%dh ago")
	set(Fr, "dlv.hoursAgo", "il y a %dh")
	set(Ar, "dlv.hoursAgo", "منذ %d س")

	set(En, "dlv.daysAgo", "%dd ago")
	set(Fr, "dlv.daysAgo", "il y a %dj")
	set(Ar, "dlv.daysAgo", "منذ %d ي")

	set(En, "dlv.watched", "watched")
	set(Fr, "dlv.watched", "suivis")
	set(Ar, "dlv.watched", "مُتتبَّع")

	set(En, "dlv.withDriver", "with a driver")
	set(Fr, "dlv.withDriver", "avec livreur")
	set(Ar, "dlv.withDriver", "مع سائق")

	set(En, "dlv.finished", "finished")
	set(Fr, "dlv.finished", "terminés")
	set(Ar, "dlv.finished", "مكتمل")

	set(En, "dlv.flashConnected", "Mes Colis connected — parcel statuses are now polled.")
	set(Fr, "dlv.flashConnected", "Mes Colis connecté — les statuts des colis sont désormais suivis.")
	set(Ar, "dlv.flashConnected", "تم ربط Mes Colis — تُفحص حالات الطرود الآن.")

	set(En, "dlv.flashTestFailed", "Connection test failed:")
	set(Fr, "dlv.flashTestFailed", "Échec du test de connexion :")
	set(Ar, "dlv.flashTestFailed", "فشل اختبار الاتصال:")

	set(En, "dlv.flashTestFailedBody", "Connection test failed — check that the access token is valid for Mes Colis Express.")
	set(Fr, "dlv.flashTestFailedBody", "Échec du test de connexion — vérifiez que le jeton est valide pour Mes Colis Express.")
	set(Ar, "dlv.flashTestFailedBody", "فشل اختبار الاتصال — تحقق من صحة رمز الوصول لـ Mes Colis Express.")

	set(En, "dlv.flashMissing", "The access token is required.")
	set(Fr, "dlv.flashMissing", "Le jeton d'accès est requis.")
	set(Ar, "dlv.flashMissing", "رمز الوصول مطلوب.")

	set(En, "dlv.flashDisconnected", "Delivery provider disconnected.")
	set(Fr, "dlv.flashDisconnected", "Prestataire de livraison déconnecté.")
	set(Ar, "dlv.flashDisconnected", "تم فصل مزوّد التوصيل.")

	set(En, "dlv.flashRemoved", "Parcel removed from tracking.")
	set(Fr, "dlv.flashRemoved", "Colis retiré du suivi.")
	set(Ar, "dlv.flashRemoved", "تمت إزالة الطرد من التتبع.")

	set(En, "dlv.flashPolledNone", "Checked %d parcel(s) — no new status changes.")
	set(Fr, "dlv.flashPolledNone", "%d colis vérifié(s) — aucun changement de statut.")
	set(Ar, "dlv.flashPolledNone", "تم فحص %d طردًا — لا تغييرات جديدة في الحالة.")

	set(En, "dlv.flashPolledChanges", "Checked %d parcel(s) — %d status change(s) found.")
	set(Fr, "dlv.flashPolledChanges", "%d colis vérifié(s) — %d changement(s) de statut détecté(s).")
	set(Ar, "dlv.flashPolledChanges", "تم فحص %d طردًا — وُجدت %d تغييرات في الحالة.")
}