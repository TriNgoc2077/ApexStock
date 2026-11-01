export interface ProductSpec {
  [key: string]: string;
}

export interface ProductReview {
  id: string;
  userName: string;
  rating: number;
  date: string;
  comment: string;
  verified: boolean;
}

export interface DetailedProductInfo {
  category: 'phone' | 'laptop' | 'audio' | 'watch' | 'keyboard';
  categoryName: string;
  rating: number;
  reviewCount: number;
  specs: ProductSpec;
  variants: {
    colors: { name: string; hex: string }[];
    options: string[];
  };
  highlights: string[];
  reviews: ProductReview[];
}

export const PRODUCT_EXTRA_DATA: Record<string, DetailedProductInfo> = {
  // iPhone 15 Pro Max
  '33333333-3333-3333-3333-333333333333': {
    category: 'phone',
    categoryName: 'Điện Thoại',
    rating: 4.9,
    reviewCount: 384,
    highlights: [
      'Khung vỏ Titanium chuẩn hàng không vũ trụ siêu nhẹ và bền bỉ',
      'Vi xử lý Apple A17 Pro tiến trình 3nm với GPU 6 lõi đẳng cấp gaming',
      'Cụm camera chính 48MP zoom quang học 5x với tiêu cự tương đương 120mm',
      'Nút Tác vụ (Action Button) tùy biến linh hoạt thay thế cần gạt rung',
      'Cổng USB-C chuẩn USB 3 cho tốc độ truyền dữ liệu tới 10Gb/s',
    ],
    variants: {
      colors: [
        { name: 'Titan Tự Nhiên', hex: '#9f9a94' },
        { name: 'Titan Xanh', hex: '#2f3b4c' },
        { name: 'Titan Đen', hex: '#3b3b3c' },
        { name: 'Titan Trắng', hex: '#e3e4e5' },
      ],
      options: ['256GB', '512GB', '1TB'],
    },
    specs: {
      'Màn hình': '6.7 inch Super Retina XDR OLED, 120Hz ProMotion, 2000 nits',
      'Bộ vi xử lý': 'Apple A17 Pro (6 nhân CPU, 6 nhân GPU)',
      'RAM & Bộ nhớ': '8GB RAM, 256GB NVMe',
      'Camera sau': 'Chính 48MP (f/1.78) + Telephoto 12MP 5x + Góc siêu rộng 12MP',
      'Camera trước': '12MP TrueDepth với Autofocus',
      'Pin & Sạc': '4,422 mAh, Sạc nhanh 27W, MagSafe 15W',
      'Hệ điều hành': 'iOS 18',
      'Kháng nước/bụi': 'IP68 (sâu 6m trong 30 phút)',
    },
    reviews: [
      {
        id: 'r1',
        userName: 'Trần Văn Hoàng (Hà Nội)',
        rating: 5,
        date: '28/10/2026',
        comment: 'Hệ thống flash sale đặt hàng cực kỳ mượt mà, bấm phát nhận ngay order và thanh toán SePay quét mã QR xong trong 3 giây. Máy cầm rất đầm tay, viền titan nhẹ hơn hẳn đời trước.',
        verified: true,
      },
      {
        id: 'r2',
        userName: 'Nguyễn Thị Thu Hà (TP.HCM)',
        rating: 5,
        date: '26/10/2026',
        comment: 'Săn deal giảm 50% đỉnh thực sự! Mình canh đúng khung giờ bấm mua không bị nghẽn mạng chút nào. Đã nhận hàng nguyên seal chính hãng VN/A.',
        verified: true,
      },
      {
        id: 'r3',
        userName: 'Đỗ Minh Tuấn',
        rating: 5,
        date: '22/10/2026',
        comment: 'Camera 5x nét căng, chụp chân dung đỉnh chóp. Khuyên mọi người nên mua bản 256GB trở lên chụp ảnh quay video thoải mái.',
        verified: true,
      },
    ],
  },

  // MacBook Pro 14 M3 Pro
  '33333333-3333-3333-3333-333333333334': {
    category: 'laptop',
    categoryName: 'Laptop & Mac',
    rating: 4.9,
    reviewCount: 192,
    highlights: [
      'Chip Apple M3 Pro mạnh mẽ kiến trúc 3nm thế hệ tiếp theo',
      'Thời lượng pin lên đến 18 giờ liên tục, sạc MagSafe 3 tiện lợi',
      'Màn hình Liquid Retina XDR 14.2 inch với độ sáng đỉnh 1600 nits',
      'Vỏ nhôm màu Space Black hạn chế tối đa bám dấu vân tay',
      'Hệ thống 6 loa âm thanh vòm Spatial Audio và 3 mic chuẩn phòng thu',
    ],
    variants: {
      colors: [
        { name: 'Space Black', hex: '#242526' },
        { name: 'Silver', hex: '#e1e3e4' },
      ],
      options: ['18GB/512GB', '18GB/1TB', '36GB/1TB'],
    },
    specs: {
      'Màn hình': '14.2 inch Liquid Retina XDR (3024x1964), 120Hz ProMotion',
      'Bộ vi xử lý': 'Apple M3 Pro (11-core CPU, 14-core GPU)',
      'RAM': '18GB Unified Memory tốc độ 150GB/s',
      'Ổ cứng': '512GB SSD PCIe tốc độ cao',
      'Cổng kết nối': '3x Thunderbolt 4, HDMI 2.1, Khe thẻ nhớ SDXC, Jack 3.5mm, MagSafe 3',
      'Pin': '70Wh Li-Po, Adapter sạc USB-C 70W',
      'Trọng lượng': '1.61 kg',
      'Hệ điều hành': 'macOS Sequoia',
    },
    reviews: [
      {
        id: 'r4',
        userName: 'Vũ Quốc Bảo (Senior Dev)',
        rating: 5,
        date: '29/10/2026',
        comment: 'Build dự án Go và Docker chỉ mất vài giây, quạt tản nhiệt hầu như không bao giờ chạy. Màn hình XDR nhìn code và thiết kế cực kỳ mãn nhãn.',
        verified: true,
      },
      {
        id: 'r5',
        userName: 'Lê Thanh Tùng',
        rating: 5,
        date: '25/10/2026',
        comment: 'Màu Space Black nhìn ngoài đời sang trọng hơn hẳn, ít bám vân tay hơn bản Midnight của Air.',
        verified: true,
      },
    ],
  },

  // Sony WH-1000XM5
  '33333333-3333-3333-3333-333333333335': {
    category: 'audio',
    categoryName: 'Tai Nghe & Âm Thanh',
    rating: 4.8,
    reviewCount: 245,
    highlights: [
      'Công nghệ chống ồn Auto NC Optimizer hàng đầu thế giới với 2 chip xử lý',
      'Driver 30mm màng loa carbon tổng hợp mang lại chất âm Hi-Res tự nhiên',
      'Thời lượng pin khủng 30 giờ khi bật ANC, sạc nhanh 3 phút nghe được 3 giờ',
      'Thiết kế quai đeo không nấc êm ái, bọc da tổng hợp mềm mại ôm sát tai',
      '8 micro kết hợp thuật toán AI khử tạp âm môi trường khi đàm thoại',
    ],
    variants: {
      colors: [
        { name: 'Đen Huyền Bí', hex: '#1c1c1c' },
        { name: 'Bạc Ánh Kim', hex: '#d9d7d2' },
        { name: 'Xanh Midnight', hex: '#1a2332' },
      ],
      options: ['Bản Tiêu Chuẩn'],
    },
    specs: {
      'Loại tai nghe': 'Over-ear không dây chống ồn chủ động (ANC)',
      'Bộ xử lý âm thanh': 'Sony Integrated Processor V1 + HD QN1',
      'Driver': '30mm màng loa carbon composite',
      'Thời lượng pin': '30 giờ (bật ANC), 40 giờ (tắt ANC)',
      'Kết nối': 'Bluetooth 5.2, LDAC, AAC, SBC, Cáp 3.5mm',
      'Tính năng': 'Speak-to-Chat, Đa điểm kết nối 2 thiết bị đồng thời',
      'Trọng lượng': '250 g',
    },
    reviews: [
      {
        id: 'r6',
        userName: 'Ngô Đức Anh',
        rating: 5,
        date: '27/10/2026',
        comment: 'Đeo đi máy bay hoặc ngồi quán cà phê ồn ào thì như ở một thế giới riêng. Âm trầm sâu, mid rõ ràng.',
        verified: true,
      },
    ],
  },

  // Apple Watch Ultra 2
  '33333333-3333-3333-3333-333333333336': {
    category: 'watch',
    categoryName: 'Đồng Hồ Thông Minh',
    rating: 4.9,
    reviewCount: 167,
    highlights: [
      'Vỏ Titanium 49mm chống va đập, kính sapphire phẳng chống trầy xước',
      'Màn hình sáng nhất từ trước tới nay 3000 nits, hiển thị rõ dưới nắng gắt',
      'Cảm biến định vị GPS tần số kép chính xác L1 và L5',
      'Đạt chuẩn chống nước WR100 và lặn độ sâu 40 mét chuẩn EN13319',
      'Nút Action cam nổi bật cho phép khởi động bài tập hoặc tính năng ngay',
    ],
    variants: {
      colors: [
        { name: 'Titanium Tự Nhiên', hex: '#9f9a94' },
      ],
      options: ['Dây Ocean Xanh', 'Dây Alpine Cam', 'Dây Trail Đen/Xám'],
    },
    specs: {
      'Kích thước màn hình': '49mm Retina OLED luôn bật, độ sáng 3000 nits',
      'Chất liệu vỏ': 'Titanium cấp hàng không vũ trụ',
      'Chipset': 'Apple S9 SiP (hỗ trợ thao tác chạm 2 lần Double Tap)',
      'Thời lượng pin': '36 giờ sử dụng thông thường, 72 giờ chế độ tiết kiệm',
      'Cảm biến': 'Đo oxy trong máu SpO2, Điện tâm đồ ECG, Nhiệt độ nước & độ sâu',
      'Định vị': 'GPS L1 + L5 tần số kép, GLONASS, Galileo, BeiDou',
    },
    reviews: [
      {
        id: 'r7',
        userName: 'Phạm Minh Trí (Triathlete)',
        rating: 5,
        date: '28/10/2026',
        comment: 'Pin trâu chạy giải marathon 42km xong pin vẫn còn hơn 70%. Bắt GPS cực kỳ nhanh và chuẩn.',
        verified: true,
      },
    ],
  },

  // iPad Pro M4
  '33333333-3333-3333-3333-333333333337': {
    category: 'phone',
    categoryName: 'Máy Tính Bảng',
    rating: 4.9,
    reviewCount: 110,
    highlights: [
      'Thiết kế mỏng kỷ lục 5.3mm - thiết bị mỏng nhất Apple từng tạo ra',
      'Đột phá với màn hình Ultra Retina XDR sử dụng công nghệ OLED hai lớp (Tandem OLED)',
      'Chip Apple M4 thế hệ mới với Neural Engine xử lý 38 nghìn tỷ phép tính/giây',
      'Hỗ trợ bút Apple Pencil Pro cảm ứng bóp (squeeze) và xoay con quay hồi chuyển',
      'Camera trước di chuyển ra viền ngang tối ưu cho gọi video và họp trực tuyến',
    ],
    variants: {
      colors: [
        { name: 'Space Black', hex: '#242526' },
        { name: 'Silver', hex: '#e1e3e4' },
      ],
      options: ['256GB Wi-Fi', '512GB Wi-Fi', '256GB 5G Cellular'],
    },
    specs: {
      'Màn hình': '11 inch Ultra Retina XDR (Tandem OLED), 120Hz ProMotion, độ sáng 1000 nits (đỉnh 1600 nits)',
      'Vi xử lý': 'Apple M4 (9 nhân CPU, 10 nhân GPU)',
      'RAM & Lưu trữ': '8GB RAM, 256GB SSD',
      'Camera': 'Sau 12MP Wide + Cảm biến LiDAR; Trước 12MP Ultra Wide viền ngang',
      'Kết nối': 'Wi-Fi 6E, Bluetooth 5.3, Thunderbolt / USB 4',
      'Độ dày & Cân nặng': '5.3 mm, 444 grams',
    },
    reviews: [
      {
        id: 'r8',
        userName: 'Hoàng Minh Châu (Graphic Designer)',
        rating: 5,
        date: '24/10/2026',
        comment: 'Màn hình Tandem OLED màu đen sâu tuyệt đối, vẽ trên Procreate với Pencil Pro có cảm giác rung haptic rất thật.',
        verified: true,
      },
    ],
  },

  // Keychron Q1 Pro
  '33333333-3333-3333-3333-333333333338': {
    category: 'keyboard',
    categoryName: 'Bàn Phím & Phụ Kiện',
    rating: 4.8,
    reviewCount: 142,
    highlights: [
      'Vỏ nhôm nguyên khối gia công CNC chính xác, trọng lượng đầm chắc',
      'Thiết kế Double-Gasket Mount mang lại âm gõ trầm ấm và cảm giác gõ êm ái',
      'Kết nối linh hoạt: Bluetooth 5.1 (3 thiết bị) và Type-C có dây polling rate 1000Hz',
      'Tương thích hoàn hảo cả macOS và Windows với switch gạt vật lý',
      'Hỗ trợ tùy biến phím và macro qua QMK/VIA trực tiếp trên trình duyệt web',
    ],
    variants: {
      colors: [
        { name: 'Carbon Black', hex: '#232323' },
        { name: 'Silver Grey', hex: '#a8a8a8' },
        { name: 'Shell White', hex: '#f0f0f0' },
      ],
      options: ['K Pro Red (Linear)', 'K Pro Brown (Tactile)', 'K Pro Banana'],
    },
    specs: {
      'Layout': '75% (81 phím + Núm xoay Knob đa năng)',
      'Chất liệu case': 'Nhôm 6063 CNC nguyên khối sơn tĩnh điện',
      'Plate': 'Polycarbonate (PC) dẻo dai',
      'Switch': 'Keychron K Pro Pre-lubed Switch, Hotswap 3-pin & 5-pin',
      'Keycap': 'KSA Profile Double-shot PBT chống bóng',
      'Dung lượng pin': '4000 mAh (dùng đến 300 giờ khi tắt đèn LED)',
      'Trọng lượng': '1.73 kg',
    },
    reviews: [
      {
        id: 'r9',
        userName: 'Đặng Tuấn Kiệt',
        rating: 5,
        date: '27/10/2026',
        comment: 'Bàn phím nặng trịch, gõ âm thock rất chắc chắn không bị rỗng. Dùng gõ code cả ngày không mỏi tay.',
        verified: true,
      },
    ],
  },

  // AirPods Pro 2
  '33333333-3333-3333-3333-333333333339': {
    category: 'audio',
    categoryName: 'Tai Nghe & Âm Thanh',
    rating: 4.9,
    reviewCount: 520,
    highlights: [
      'Chip H2 mang lại khả năng chống ồn chủ động gấp đôi thế hệ trước',
      'Âm thanh Thích Ứng (Adaptive Audio) tự động điều chỉnh âm lượng theo môi trường',
      'Hộp sạc MagSafe cổng USB-C tích hợp loa định vị và chip tìm kiếm chính xác U1',
      'Tính năng Nhận Biết Cuộc Hội Thoại tự động giảm nhỏ nhạc khi bạn cất lời',
      'Thời lượng nghe lên tới 6 giờ liên tục (30 giờ kèm hộp sạc)',
    ],
    variants: {
      colors: [{ name: 'Trắng Tinh Khiết', hex: '#ffffff' }],
      options: ['Bản MagSafe USB-C'],
    },
    specs: {
      'Loại': 'In-ear không dây True Wireless với 4 cặp đệm tai silicon (XS, S, M, L)',
      'Chip': 'Apple H2 trong tai nghe, Apple U1 trong hộp sạc',
      'Chống nước': 'IP54 cho cả tai nghe và hộp sạc',
      'Cổng sạc': 'USB-C, Hỗ trợ sạc MagSafe và Qi',
      'Thời lượng pin': '6 giờ 1 lần sạc, tổng 30 giờ với case',
    },
    reviews: [
      {
        id: 'r10',
        userName: 'Lê Gia Huy',
        rating: 5,
        date: '30/10/2026',
        comment: 'Cổng Type-C đồng bộ tiện với dây sạc iPhone 15 và MacBook. Khử tiếng ồn đỉnh của chóp.',
        verified: true,
      },
    ],
  },

  // Galaxy S24 Ultra
  '33333333-3333-3333-3333-333333333340': {
    category: 'phone',
    categoryName: 'Điện Thoại',
    rating: 4.8,
    reviewCount: 310,
    highlights: [
      'Bộ công cụ Galaxy AI tiên phong: Phiên dịch viên trực tiếp, Khoanh tròn để tìm kiếm',
      'Khung viền Titan bền bỉ kết hợp kính cường lực chống chói Corning Gorilla Armor',
      'Màn hình phẳng Dynamic AMOLED 2X 6.8 inch 2600 nits cực nét',
      'Bút S Pen huyền thoại tích hợp ngay trong thân máy tiện ghi chú và vẽ',
      'Hệ thống 4 camera sau với cảm biến chính 200MP và zoom quang học 5x chất lượng 10x',
    ],
    variants: {
      colors: [
        { name: 'Xám Titan', hex: '#58595b' },
        { name: 'Đen Titan', hex: '#2b2b2b' },
        { name: 'Tím Titan', hex: '#4e4c63' },
        { name: 'Vàng Titan', hex: '#d4cebe' },
      ],
      options: ['12GB/256GB', '12GB/512GB', '12GB/1TB'],
    },
    specs: {
      'Màn hình': '6.8 inch Dynamic AMOLED 2X, QHD+, 1-120Hz, Gorilla Armor chống phản chiếu',
      'Bộ vi xử lý': 'Snapdragon 8 Gen 3 for Galaxy (4nm)',
      'RAM & Bộ nhớ': '12GB RAM LPDDR5X, 256GB UFS 4.0',
      'Camera sau': '200MP (f/1.7) + 50MP Periscope 5x + 10MP Tele 3x + 12MP Ultra-wide',
      'Camera trước': '12MP Dual Pixel AF',
      'Pin & Sạc': '5,000 mAh, Sạc có dây 45W, Không dây 15W',
      'Hệ điều hành': 'Android 14 với One UI 6.1 (Cam kết 7 năm cập nhật OS)',
    },
    reviews: [
      {
        id: 'r11',
        userName: 'Huỳnh Nhật Nam',
        rating: 5,
        date: '28/10/2026',
        comment: 'Kính Gorilla Armor chống chói thật sự bá đạo, ra nắng gắt xem phim không hề bị lóa bóng. AI Circle to search dùng nghiện luôn.',
        verified: true,
      },
    ],
  },
};

export const DEFAULT_PRODUCT_INFO: DetailedProductInfo = {
  category: 'phone',
  categoryName: 'Sản Phẩm Công Nghệ',
  rating: 4.9,
  reviewCount: 150,
  highlights: [
    'Hàng chính hãng phân phối ủy quyền đầy đủ hóa đơn VAT',
    'Bảo hành 12 tháng tại các trung tâm dịch vụ ủy quyền toàn quốc',
    'Chính sách 1 đổi 1 trong 30 ngày nếu phát sinh lỗi nhà sản xuất',
    'Hỗ trợ giao hàng hỏa tốc trong 2 giờ',
  ],
  variants: {
    colors: [{ name: 'Tiêu Chuẩn', hex: '#3b82f6' }],
    options: ['Phiên bản tiêu chuẩn'],
  },
  specs: {
    'Tình trạng': 'Mới 100% nguyên hộp seal',
    'Phụ kiện': 'Thân máy, Cáp sạc, Sách hướng dẫn sử dụng',
    'Bảo hành': '12 tháng chính hãng',
  },
  reviews: [],
};
