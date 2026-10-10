package i18n

// Core interface strings: the language switcher, navigation, shared layout,
// authentication, the public landing/legal chrome and error pages.
func init() {
	// --- language switcher ---
	set(En, "switcher.label", "Change language")
	set(Fr, "switcher.label", "Changer de langue")
	set(Ar, "switcher.label", "تغيير اللغة")

	// --- navigation ---
	set(En, "nav.overview", "Overview")
	set(Fr, "nav.overview", "Aperçu")
	set(Ar, "nav.overview", "نظرة عامة")

	set(En, "nav.integrations", "Shop Integration")
	set(Fr, "nav.integrations", "Intégration boutique")
	set(Ar, "nav.integrations", "ربط المتجر")

	set(En, "nav.automations", "Automations")
	set(Fr, "nav.automations", "Automatisations")
	set(Ar, "nav.automations", "الأتمتة")

	set(En, "nav.templates", "Templates")
	set(Fr, "nav.templates", "Modèles")
	set(Ar, "nav.templates", "القوالب")

	set(En, "nav.inbox", "Inbox")
	set(Fr, "nav.inbox", "Boîte de réception")
	set(Ar, "nav.inbox", "الوارد")

	set(En, "nav.delivery", "Delivery")
	set(Fr, "nav.delivery", "Livraison")
	set(Ar, "nav.delivery", "التوصيل")

	set(En, "nav.settings", "Settings")
	set(Fr, "nav.settings", "Paramètres")
	set(Ar, "nav.settings", "الإعدادات")

	// --- shared layout ---
	set(En, "layout.lockShop", "Connect your shop to unlock this")
	set(Fr, "layout.lockShop", "Connectez votre boutique pour débloquer cette section")
	set(Ar, "layout.lockShop", "اربط متجرك لفتح هذا القسم")

	set(En, "layout.lockWhatsapp", "Connect your WhatsApp number to unlock this")
	set(Fr, "layout.lockWhatsapp", "Connectez votre numéro WhatsApp pour débloquer cette section")
	set(Ar, "layout.lockWhatsapp", "اربط رقم واتساب لفتح هذا القسم")

	set(En, "layout.noShop", "No shop connected yet.")
	set(Fr, "layout.noShop", "Aucune boutique connectée.")
	set(Ar, "layout.noShop", "لا يوجد متجر متصل بعد.")

	set(En, "layout.noShopBody", "Overview, Automations and Templates unlock once you connect a Converty store.")
	set(Fr, "layout.noShopBody", "Les pages Aperçu, Automatisations et Modèles se débloquent dès que vous connectez une boutique Converty.")
	set(Ar, "layout.noShopBody", "تُفتح صفحات نظرة عامة والأتمتة والقوالب بمجرد ربط متجر Converty.")

	set(En, "layout.connectStore", "Connect a store")
	set(Fr, "layout.connectStore", "Connecter une boutique")
	set(Ar, "layout.connectStore", "ربط متجر")

	set(En, "layout.openMenu", "Open menu")
	set(Fr, "layout.openMenu", "Ouvrir le menu")
	set(Ar, "layout.openMenu", "فتح القائمة")

	// --- authentication ---
	set(En, "auth.signIn", "Sign in")
	set(Fr, "auth.signIn", "Se connecter")
	set(Ar, "auth.signIn", "تسجيل الدخول")

	set(En, "auth.signOut", "Sign out")
	set(Fr, "auth.signOut", "Se déconnecter")
	set(Ar, "auth.signOut", "تسجيل الخروج")

	set(En, "auth.email", "Email")
	set(Fr, "auth.email", "E-mail")
	set(Ar, "auth.email", "البريد الإلكتروني")

	set(En, "auth.password", "Password")
	set(Fr, "auth.password", "Mot de passe")
	set(Ar, "auth.password", "كلمة المرور")

	set(En, "auth.tagline", "Automated order notifications for your shop")
	set(Fr, "auth.tagline", "Notifications de commande automatisées pour votre boutique")
	set(Ar, "auth.tagline", "إشعارات الطلبات الآلية لمتجرك")

	set(En, "auth.badCredentials", "incorrect email or password")
	set(Fr, "auth.badCredentials", "e-mail ou mot de passe incorrect")
	set(Ar, "auth.badCredentials", "البريد الإلكتروني أو كلمة المرور غير صحيحة")

	set(En, "auth.emailInvalid", "is not a valid email address")
	set(Fr, "auth.emailInvalid", "n'est pas une adresse e-mail valide")
	set(Ar, "auth.emailInvalid", "ليس عنوان بريد إلكتروني صالح")

	// --- landing ---
	set(En, "landing.brandTitle", "Converty WhatsApp")
	set(Fr, "landing.brandTitle", "Converty WhatsApp")
	set(Ar, "landing.brandTitle", "Converty WhatsApp")

	set(En, "landing.heading", "Automated WhatsApp order notifications")
	set(Fr, "landing.heading", "Notifications de commande WhatsApp automatisées")
	set(Ar, "landing.heading", "إشعارات طلبات واتساب الآلية")

	set(En, "landing.body", "Connect your WhatsApp Business number, configure templates and let Converty order statuses drive the messages your customers receive.")
	set(Fr, "landing.body", "Connectez votre numéro WhatsApp Business, configurez vos modèles et laissez les statuts de commande Converty déclencher les messages reçus par vos clients.")
	set(Ar, "landing.body", "اربط رقم واتساب الأعمال، وهيئ القوالب، ودع حالات طلبات Converty تتحكم في الرسائل التي يستلمها عملاؤك.")

	set(En, "landing.provisioned", "Accounts are created by our team — this service is provisioned per merchant.")
	set(Fr, "landing.provisioned", "Les comptes sont créés par notre équipe — ce service est provisionné marchand par marchand.")
	set(Ar, "landing.provisioned", "يُنشئ فريقنا الحسابات — تُهيَّأ هذه الخدمة لكل تاجر على حدة.")

	// --- legal chrome ---
	set(En, "legal.privacy", "Privacy Policy")
	set(Fr, "legal.privacy", "Politique de confidentialité")
	set(Ar, "legal.privacy", "سياسة الخصوصية")

	set(En, "legal.deletion", "Data deletion")
	set(Fr, "legal.deletion", "Suppression des données")
	set(Ar, "legal.deletion", "حذف البيانات")

	set(En, "legal.deletionLong", "Data Deletion Instructions")
	set(Fr, "legal.deletionLong", "Instructions de suppression des données")
	set(Ar, "legal.deletionLong", "تعليمات حذف البيانات")

	set(En, "legal.backHome", "← Back to home")
	set(Fr, "legal.backHome", "← Retour à l'accueil")
	set(Ar, "legal.backHome", "→ العودة إلى الرئيسية")

	set(En, "legal.home", "Home")
	set(Fr, "legal.home", "Accueil")
	set(Ar, "legal.home", "الرئيسية")

	set(En, "legal.contactSend", "Send your request to")
	set(Fr, "legal.contactSend", "Envoyez votre demande à")
	set(Ar, "legal.contactSend", "أرسل طلبك إلى")

	set(En, "legal.contactSubject", "in the subject line, and include the shop name or the customer phone number the request relates to.")
	set(Fr, "legal.contactSubject", "en objet, et précisez le nom de la boutique ou le numéro de téléphone du client concerné.")
	set(Ar, "legal.contactSubject", "في سطر الموضوع، وأدرج اسم المتجر أو رقم هاتف العميل المعني بالطلب.")

	set(En, "legal.contactSendTeam", "Send your request to our support team with")
	set(Fr, "legal.contactSendTeam", "Envoyez votre demande à notre équipe d'assistance avec")
	set(Ar, "legal.contactSendTeam", "أرسل طلبك إلى فريق الدعم مع")

	// --- errors ---
	set(En, "error.goHome", "Go home")
	set(Fr, "error.goHome", "Retour à l'accueil")
	set(Ar, "error.goHome", "العودة إلى الرئيسية")

	set(En, "error.notFoundTitle", "Page not found")
	set(Fr, "error.notFoundTitle", "Page introuvable")
	set(Ar, "error.notFoundTitle", "الصفحة غير موجودة")

	set(En, "error.notFoundBody", "The page you're looking for doesn't exist.")
	set(Fr, "error.notFoundBody", "La page que vous recherchez n'existe pas.")
	set(Ar, "error.notFoundBody", "الصفحة التي تبحث عنها غير موجودة.")

	set(En, "error.serverErrorTitle", "Something went wrong")
	set(Fr, "error.serverErrorTitle", "Une erreur est survenue")
	set(Ar, "error.serverErrorTitle", "حدث خطأ ما")

	set(En, "error.unauthorizedTitle", "Unauthorized")
	set(Fr, "error.unauthorizedTitle", "Non autorisé")
	set(Ar, "error.unauthorizedTitle", "غير مصرّح")

	set(En, "error.genericBody", "Something went wrong. Please try again.")
	set(Fr, "error.genericBody", "Une erreur est survenue. Veuillez réessayer.")
	set(Ar, "error.genericBody", "حدث خطأ ما. يرجى المحاولة مجددًا.")

	// --- page titles (rendered in the topbar / document title) ---
	set(En, "title.overview", "Overview")
	set(Fr, "title.overview", "Aperçu")
	set(Ar, "title.overview", "نظرة عامة")

	set(En, "title.shopIntegration", "Shop Integration")
	set(Fr, "title.shopIntegration", "Intégration boutique")
	set(Ar, "title.shopIntegration", "ربط المتجر")

	set(En, "title.automations", "Automations")
	set(Fr, "title.automations", "Automatisations")
	set(Ar, "title.automations", "الأتمتة")

	set(En, "title.sendHistory", "Send history")
	set(Fr, "title.sendHistory", "Historique des envois")
	set(Ar, "title.sendHistory", "سجل الإرسال")

	set(En, "title.automation", "Automation")
	set(Fr, "title.automation", "Automatisation")
	set(Ar, "title.automation", "أتمتة")

	set(En, "title.templates", "Templates")
	set(Fr, "title.templates", "Modèles")
	set(Ar, "title.templates", "القوالب")

	set(En, "title.inbox", "Inbox")
	set(Fr, "title.inbox", "Boîte de réception")
	set(Ar, "title.inbox", "الوارد")

	set(En, "title.delivery", "Delivery")
	set(Fr, "title.delivery", "Livraison")
	set(Ar, "title.delivery", "التوصيل")

	set(En, "title.settings", "Settings")
	set(Fr, "title.settings", "Paramètres")
	set(Ar, "title.settings", "الإعدادات")

	set(En, "title.enableWhatsapp", "Enable WhatsApp")
	set(Fr, "title.enableWhatsapp", "Activer WhatsApp")
	set(Ar, "title.enableWhatsapp", "تفعيل واتساب")

	// --- generic ---
	set(En, "common.comingSoon", "Coming soon")
	set(Fr, "common.comingSoon", "Bientôt disponible")
	set(Ar, "common.comingSoon", "قريبًا")
}