// Webs de pago que meten los campos de la tarjeta en un marco propio dentro de
// la tienda (Stripe, Adyen, Redsys…). En esos marcos TurboVAULT solo rellena
// tarjetas, nunca contraseñas, y solo si van por https. La usan la parte de las
// páginas (content.js) y el fondo (background.js).
self.TV_PAY = (() => {
  const HOSTS = [
    'stripe.com', 'stripe.network', // Stripe Elements
    'adyen.com', 'adyenpayments.com', // Adyen
    'braintreegateway.com', 'braintree-api.com', // Braintree (PayPal)
    'paypal.com', // campos de tarjeta de PayPal
    'checkout.com', // Checkout.com Frames
    'redsys.es', // Redsys InSite (TPV de los bancos españoles)
    'ceca.es', // TPV de CECA
    'paycomet.com', 'monei.com', 'addonpayments.com', // pasarelas españolas
    'mollie.com', 'squarecdn.com', 'squareup.com', 'recurly.com', 'chargebee.com', 'paddle.com',
    'shopifyinc.com', 'shopifycs.com', // pago de las tiendas Shopify
    'worldpay.com', 'authorize.net', 'cybersource.com', 'globalpay.com', 'realexpayments.com',
    'payu.com', 'mercadopago.com', 'nuvei.com', 'safecharge.com', 'rapyd.net', 'payplug.com',
    'mangopay.com', 'spreedly.com', 'basistheory.com', 'sumup.com', 'revolut.com',
  ];
  const isPayHost = (h) => {
    h = String(h || '').toLowerCase().replace(/\.$/, '');
    return HOSTS.some((d) => h === d || h.endsWith('.' + d));
  };
  return { HOSTS, isPayHost };
})();
