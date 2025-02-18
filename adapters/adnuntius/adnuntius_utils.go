package adnuntius

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/prebid/openrtb/v20/openrtb2"
	"github.com/prebid/prebid-server/v3/openrtb_ext"
	"github.com/prebid/prebid-server/v3/util/jsonutil"
)

func setHeaders(ortbRequest openrtb2.BidRequest) http.Header {

	headers := http.Header{}
	headers.Add("Content-Type", "application/json;charset=utf-8")
	headers.Add("Accept", "application/json")
	if ortbRequest.Device != nil {
		if ortbRequest.Device.IP != "" {
			headers.Add("X-Forwarded-For", ortbRequest.Device.IP)
		}
		if ortbRequest.Device.UA != "" {
			headers.Add("user-agent", ortbRequest.Device.UA)
		}
	}
	return headers
}

func makeEndpointUrl(ortbRequest openrtb2.BidRequest, a *adapter, noCookies bool) (string, []error) {
	uri, err := url.Parse(a.endpoint)
	endpointUrl := a.endpoint
	if err != nil {
		return "", []error{fmt.Errorf("failed to parse Adnuntius endpoint: %v", err)}
	}

	gdpr, consent, err := getGDPR(&ortbRequest)
	if err != nil {
		return "", []error{fmt.Errorf("failed to parse Adnuntius endpoint: %v", err)}
	}

	if !noCookies {
		var deviceExt extDeviceAdnuntius
		if ortbRequest.Device != nil && ortbRequest.Device.Ext != nil {
			if err := jsonutil.Unmarshal(ortbRequest.Device.Ext, &deviceExt); err != nil {
				return "", []error{fmt.Errorf("failed to parse Adnuntius endpoint: %v", err)}
			}
		}

		if deviceExt.NoCookies {
			noCookies = true
		}
	}

	_, offset := a.time.Now().Zone()
	tzo := -offset / minutesInHour

	q := uri.Query()
	if gdpr != "" {
		endpointUrl = a.extraInfo
		q.Set("gdpr", gdpr)
	}

	if consent != "" {
		q.Set("consentString", consent)
	}

	if noCookies {
		q.Set("noCookies", "true")
	}

	q.Set("tzo", fmt.Sprint(tzo))
	q.Set("format", "prebidServer")

	url := endpointUrl + "?" + q.Encode()
	return url, nil
}

func getImpSizes(imp openrtb2.Imp) [][]int64 {

	if len(imp.Banner.Format) > 0 {
		sizes := make([][]int64, len(imp.Banner.Format))
		for i, format := range imp.Banner.Format {
			sizes[i] = []int64{format.W, format.H}
		}

		return sizes
	}

	if imp.Banner.W != nil && imp.Banner.H != nil {
		size := make([][]int64, 1)
		size[0] = []int64{*imp.Banner.W, *imp.Banner.H}
		return size
	}

	return nil
}

func getSiteExtAsKv(request *openrtb2.BidRequest) (siteExt, error) {
	var extSite siteExt
	if request.Site != nil && request.Site.Ext != nil {
		if err := jsonutil.Unmarshal(request.Site.Ext, &extSite); err != nil {
			return extSite, fmt.Errorf("failed to parse ExtSite in Adnuntius: %v", err)
		}
	}
	return extSite, nil
}

func getGDPR(request *openrtb2.BidRequest) (string, string, error) {

	gdpr := ""
	var extRegs openrtb_ext.ExtRegs
	if request.Regs != nil && request.Regs.Ext != nil {
		if err := jsonutil.Unmarshal(request.Regs.Ext, &extRegs); err != nil {
			return "", "", fmt.Errorf("failed to parse ExtRegs in Adnuntius GDPR check: %v", err)
		}
		if extRegs.GDPR != nil && (*extRegs.GDPR == 0 || *extRegs.GDPR == 1) {
			gdpr = strconv.Itoa(int(*extRegs.GDPR))
		}
	}

	consent := ""
	if request.User != nil && request.User.Ext != nil {
		var extUser openrtb_ext.ExtUser
		if err := jsonutil.Unmarshal(request.User.Ext, &extUser); err != nil {
			return "", "", fmt.Errorf("failed to parse ExtUser in Adnuntius GDPR check: %v", err)
		}
		consent = extUser.Consent
	}

	return gdpr, consent, nil
}

func generateReturnExt(ad Ad, request *openrtb2.BidRequest) (json.RawMessage, error) {
	// We always force the publisher to render
	var adRender int8 = 0

	var requestRegsExt *openrtb_ext.ExtRegs
	if request.Regs != nil && request.Regs.Ext != nil {
		if err := jsonutil.Unmarshal(request.Regs.Ext, &requestRegsExt); err != nil {

			return nil, fmt.Errorf("Failed to parse Ext information in Adnuntius: %v", err)
		}
	}

	if ad.Advertiser.Name != "" && requestRegsExt != nil && requestRegsExt.DSA != nil {
		legalName := ad.Advertiser.Name
		if ad.Advertiser.LegalName != "" {
			legalName = ad.Advertiser.LegalName
		}
		ext := &openrtb_ext.ExtBid{
			DSA: &openrtb_ext.ExtBidDSA{
				AdRender: &adRender,
				Paid:     legalName,
				Behalf:   legalName,
			},
		}

		returnExt, err := json.Marshal(ext)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse Ext information in Adnuntius: %v", err)
		}

		return returnExt, nil
	}
	return nil, nil
}

func generateAdUnit(imp openrtb2.Imp, adnuntiusExt openrtb_ext.ImpExtAdnunitus, bidType string) adnRequestAdunit {
	var adUnit adnRequestAdunit
	adUnit = adnRequestAdunit{
		AuId:       adnuntiusExt.Auid,
		TargetId:   fmt.Sprintf("%s-%s:%s", adnuntiusExt.Auid, imp.ID, bidType),
		Dimensions: getImpSizes(imp),
	}
	if adnuntiusExt.MaxDeals > 0 {
		adUnit.MaxDeals = adnuntiusExt.MaxDeals
	}
	return adUnit
}

/*
1 = Banner
2 = Video
3 = Audio
4 = Native
*/
func convertMarkupTypeToBidType(markupType openrtb2.MarkupType) openrtb_ext.BidType {
	switch markupType {
	case 1:
		return openrtb_ext.BidTypeBanner
	case 2:
		return openrtb_ext.BidTypeVideo
	case 3:
		return openrtb_ext.BidTypeAudio
	case 4:
		return openrtb_ext.BidTypeNative
	}
	return openrtb_ext.BidTypeBanner
}
